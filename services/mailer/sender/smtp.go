// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package sender

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"os"
	"strings"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"

	"github.com/wneessen/go-mail/smtp"
)

// SMTPSender Sender SMTP mail sender
type SMTPSender struct{}

var _ Sender = &SMTPSender{}

// Send send email
func (s *SMTPSender) Send(from string, to []string, msg io.WriterTo) error {
	opts := setting.MailService

	var network string
	var address string
	if opts.Protocol == "smtp+unix" {
		network = "unix"
		address = opts.SMTPAddr
	} else {
		network = "tcp"
		address = net.JoinHostPort(opts.SMTPAddr, opts.SMTPPort)
	}

	conn, err := net.Dial(network, address)
	if err != nil {
		return fmt.Errorf("failed to establish network connection to SMTP server: %w", err)
	}
	defer conn.Close()

	var tlsconfig *tls.Config
	if opts.Protocol == "smtps" || opts.Protocol == "smtp+starttls" {
		tlsconfig = &tls.Config{
			InsecureSkipVerify: opts.ForceTrustServerCert,
			ServerName:         opts.SMTPAddr,
		}

		if opts.UseClientCert {
			cert, err := tls.LoadX509KeyPair(opts.ClientCertFile, opts.ClientKeyFile)
			if err != nil {
				return fmt.Errorf("could not load SMTP client certificate: %w", err)
			}
			tlsconfig.Certificates = []tls.Certificate{cert}
		}
	}

	if opts.Protocol == "smtps" {
		conn = tls.Client(conn, tlsconfig)
	}

	host := "localhost"
	if opts.Protocol == "smtp+unix" {
		host = opts.SMTPAddr
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("could not initiate SMTP session: %w", err)
	}

	if opts.EnableHelo {
		hostname := opts.HeloHostname
		if len(hostname) == 0 {
			hostname, err = os.Hostname()
			if err != nil {
				return fmt.Errorf("could not retrieve system hostname: %w", err)
			}
		}

		if err = client.Hello(hostname); err != nil {
			return fmt.Errorf("failed to issue HELO command: %w", err)
		}
	}

	if opts.Protocol == "smtp+starttls" {
		hasStartTLS, _ := client.Extension("STARTTLS")
		if hasStartTLS {
			if err = client.StartTLS(tlsconfig); err != nil {
				return fmt.Errorf("failed to start TLS connection: %w", err)
			}
		} else {
			log.Warn("StartTLS requested, but SMTP server does not support it; falling back to regular SMTP")
		}
	}

	canAuth, options := client.Extension("AUTH")
	if len(opts.User) > 0 {
		if !canAuth {
			return errors.New("SMTP server does not support AUTH, but credentials provided")
		}

		var auth smtp.Auth

		if strings.Contains(options, "CRAM-MD5") {
			auth = smtp.CRAMMD5Auth(opts.User, opts.Passwd)
		} else if strings.Contains(options, "PLAIN") {
			auth = smtp.PlainAuth("", opts.User, opts.Passwd, host, false)
		} else if strings.Contains(options, "LOGIN") {
			// Patch for AUTH LOGIN
			auth = LoginAuth(opts.User, opts.Passwd)
		} else if strings.Contains(options, "NTLM") {
			auth = NtlmAuth(opts.User, opts.Passwd)
		}

		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return fmt.Errorf("failed to authenticate SMTP: %w", err)
			}
		}
	}

	// Since go-mail 0.7.1 (CVE-2025-59937) the smtp package sends addresses verbatim,
	// so they must be validated and wrapped in angle brackets here to keep user-controlled
	// input from injecting ESMTP parameters.
	fromAddr := from
	if opts.OverrideEnvelopeFrom {
		fromAddr = opts.EnvelopeFrom
	}
	smtpFrom := "<>" // an empty override is the null reverse-path (ENVELOPE_FROM = <>)
	if fromAddr != "" {
		addr, err := sanitizeEmailAddress(fromAddr)
		if err != nil {
			return fmt.Errorf("invalid envelope from address: %w", err)
		}
		smtpFrom = "<" + addr + ">"
	}
	if err = client.Mail(smtpFrom); err != nil {
		return fmt.Errorf("failed to issue MAIL command: %w", err)
	}

	for _, rec := range to {
		addr, err := sanitizeEmailAddress(rec)
		if err != nil {
			return fmt.Errorf("invalid recipient address %q: %w", rec, err)
		}
		if err = client.Rcpt("<" + addr + ">"); err != nil {
			return fmt.Errorf("failed to issue RCPT command: %w", err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to issue DATA command: %w", err)
	} else if _, err = msg.WriteTo(w); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	} else if err = w.Close(); err != nil {
		return fmt.Errorf("SMTP close failed: %w", err)
	}

	err = client.Quit()
	if err != nil {
		log.Error("Quit client failed: %v", err)
	}

	return nil
}

// sanitizeEmailAddress parses raw as a single RFC 5322 address and returns the bare addr-spec,
// rejecting trailing data such as ESMTP parameters.
func sanitizeEmailAddress(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return addr.Address, nil
}

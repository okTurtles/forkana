// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package sender

import (
	"net"
	"net/textproto"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeEmailAddress(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "a@example.com", want: "a@example.com"},
		{raw: "<a@example.com>", want: "a@example.com"},
		{raw: "  a@example.com  ", want: "a@example.com"},
		{raw: "Alice <a@example.com>", want: "a@example.com"},
		{raw: "", wantErr: true},
		{raw: "not-an-address", wantErr: true},
		{raw: "a@example.com> SIZE=1", wantErr: true},
		{raw: "a@example.com NOTIFY=NEVER", wantErr: true},
		{raw: "a@example.com>,<b@example.com", wantErr: true},
		{raw: `"ab"@example.com`, want: "ab@example.com"},
		{raw: "a@[1.2.3.4]", want: "a@[1.2.3.4]"},
		// quoted local parts that net/mail would unquote into an unsafe SMTP path
		{raw: `"a> SIZE=1"@example.com`, wantErr: true},
		{raw: `"a b"@example.com`, wantErr: true},
		{raw: `"a\"b"@example.com`, wantErr: true},
	}
	for _, c := range cases {
		got, err := sanitizeEmailAddress(c.raw)
		if c.wantErr {
			assert.Error(t, err, "raw=%q", c.raw)
			continue
		}
		require.NoError(t, err, "raw=%q", c.raw)
		assert.Equal(t, c.want, got, "raw=%q", c.raw)
	}
}

// fakeSMTPServer accepts a single SMTP session and sends every client command line to the returned channel.
func fakeSMTPServer(t *testing.T) (port string, commands <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	ch := make(chan string, 32)
	go func() {
		defer close(ch)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tp := textproto.NewConn(conn)
		_ = tp.PrintfLine("220 localhost ESMTP")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			ch <- line
			switch verb, _, _ := strings.Cut(strings.ToUpper(line), " "); verb {
			case "EHLO", "HELO", "RSET", "NOOP":
				_ = tp.PrintfLine("250 localhost")
			case "MAIL", "RCPT":
				_ = tp.PrintfLine("250 OK")
			case "DATA":
				_ = tp.PrintfLine("354 go ahead")
				if _, err := tp.ReadDotBytes(); err != nil {
					return
				}
				_ = tp.PrintfLine("250 queued")
			case "QUIT":
				_ = tp.PrintfLine("221 bye")
				return
			default:
				_ = tp.PrintfLine("502 not implemented")
			}
		}
	}()
	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, ch
}

func TestSMTPSenderEnvelope(t *testing.T) {
	cases := []struct {
		name         string
		envelopeFrom string
		override     bool
		wantMail     string
		wantErr      string
	}{
		{name: "message from", wantMail: "MAIL FROM:<from@example.com>"},
		{name: "override", envelopeFrom: "bounce@example.com", override: true, wantMail: "MAIL FROM:<bounce@example.com>"},
		{name: "null reverse-path", override: true, wantMail: "MAIL FROM:<>"},
		{name: "invalid override", envelopeFrom: "not-an-address", override: true, wantErr: "invalid envelope from address"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port, commands := fakeSMTPServer(t)
			defer test.MockVariableValue(&setting.MailService, &setting.Mailer{
				Protocol:             "smtp",
				SMTPAddr:             "127.0.0.1",
				SMTPPort:             port,
				EnvelopeFrom:         c.envelopeFrom,
				OverrideEnvelopeFrom: c.override,
			})()

			msg := NewMessageFrom("to@example.com", "Sender", "from@example.com", "subject", "body").ToMessage()
			err := (&SMTPSender{}).Send("from@example.com", []string{"to@example.com", "Bob <bob@example.com>"}, msg)
			if c.wantErr != "" {
				require.ErrorContains(t, err, c.wantErr)
				for line := range commands {
					assert.NotRegexp(t, `^(MAIL|RCPT) `, line, "no envelope command should reach the server")
				}
				return
			}
			require.NoError(t, err)

			var got []string
			for line := range commands {
				if strings.HasPrefix(line, "MAIL ") || strings.HasPrefix(line, "RCPT ") {
					got = append(got, line)
				}
			}
			assert.Equal(t, []string{c.wantMail, "RCPT TO:<to@example.com>", "RCPT TO:<bob@example.com>"}, got)
		})
	}
}

func TestSMTPSenderRejectsInjectedRecipient(t *testing.T) {
	port, commands := fakeSMTPServer(t)
	defer test.MockVariableValue(&setting.MailService, &setting.Mailer{Protocol: "smtp", SMTPAddr: "127.0.0.1", SMTPPort: port})()

	msg := NewMessage("to@example.com", "subject", "body").ToMessage()
	err := (&SMTPSender{}).Send("from@example.com", []string{"to@example.com> NOTIFY=NEVER"}, msg)
	require.ErrorContains(t, err, "invalid recipient address")

	for line := range commands {
		assert.NotContains(t, line, "RCPT", "no RCPT command should reach the server")
	}
}

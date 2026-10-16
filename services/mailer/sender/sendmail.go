// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package sender

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/process"
	"code.gitea.io/gitea/modules/setting"
)

// sendmailStderrLimit caps how much of the sendmail program's stderr is kept for the error message
const sendmailStderrLimit = 4096

// limitedBuffer keeps the first max bytes written to it and silently drops the rest,
// so a misbehaving sendmail can neither grow memory without bound nor be blocked on stderr
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if remaining := b.max - b.buf.Len(); remaining > 0 {
		b.buf.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

// SendmailSender Sender sendmail mail sender
type SendmailSender struct{}

var _ Sender = &SendmailSender{}

// Send send email
func (s *SendmailSender) Send(from string, to []string, msg io.WriterTo) error {
	var err error
	var closeError error
	var waitError error

	envelopeFrom := from
	if setting.MailService.OverrideEnvelopeFrom {
		envelopeFrom = setting.MailService.EnvelopeFrom
	}

	args := []string{"-f", envelopeFrom, "-i"}
	args = append(args, setting.MailService.SendmailArgs...)
	args = append(args, to...)
	log.Trace("Sending with: %s %v", setting.MailService.SendmailPath, args)

	desc := fmt.Sprintf("SendMail: %s %v", setting.MailService.SendmailPath, args)

	ctx, _, finished := process.GetManager().AddContextTimeout(graceful.GetManager().HammerContext(), setting.MailService.SendmailTimeout, desc)
	defer finished()

	cmd := exec.CommandContext(ctx, setting.MailService.SendmailPath, args...)
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	// Without this, a failing sendmail (e.g. msmtp) is only reported as its exit status
	stderr := &limitedBuffer{max: sendmailStderrLimit}
	cmd.Stderr = stderr
	process.SetSysProcAttribute(cmd)

	if err = cmd.Start(); err != nil {
		_ = pipe.Close()
		return err
	}

	if setting.MailService.SendmailConvertCRLF {
		buf := &strings.Builder{}
		_, err = msg.WriteTo(buf)
		if err == nil {
			_, err = strings.NewReplacer("\r\n", "\n").WriteString(pipe, buf.String())
		}
	} else {
		_, err = msg.WriteTo(pipe)
	}

	// we MUST close the pipe or sendmail will hang waiting for more of the message
	// Also we should wait on our sendmail command even if something fails
	closeError = pipe.Close()
	waitError = cmd.Wait()
	if err != nil {
		return err
	} else if closeError != nil {
		return closeError
	}
	if waitError != nil {
		if output := strings.TrimSpace(stderr.String()); output != "" {
			return fmt.Errorf("%w: %s", waitError, output)
		}
	}
	return waitError
}

// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package sender

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSendmailScript writes an executable shell script standing in for msmtp
func writeSendmailScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sendmail")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755))
	return path
}

func TestSendmailSender(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sendmail scripts need a POSIX shell")
	}

	t.Run("Success", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		script := writeSendmailScript(t, `echo "$@" > "`+out+`.args"; cat > "`+out+`"`)
		defer test.MockVariableValue(&setting.MailService, &setting.Mailer{
			SendmailPath:        script,
			SendmailArgs:        []string{"--file=/etc/gitea/msmtprc"},
			SendmailTimeout:     time.Minute,
			SendmailConvertCRLF: true,
		})()

		err := (&SendmailSender{}).Send("from@example.com", []string{"to@example.com"}, strings.NewReader("Subject: hi\r\n\r\nbody\r\n"))
		require.NoError(t, err)

		args, err := os.ReadFile(out + ".args")
		require.NoError(t, err)
		assert.Equal(t, "-f from@example.com -i --file=/etc/gitea/msmtprc to@example.com\n", string(args))

		msg, err := os.ReadFile(out)
		require.NoError(t, err)
		assert.Equal(t, "Subject: hi\n\nbody\n", string(msg))
	})

	t.Run("FailureIncludesStderr", func(t *testing.T) {
		script := writeSendmailScript(t, "cat > /dev/null; echo 'msmtp: cannot connect to relay' >&2; exit 69")
		defer test.MockVariableValue(&setting.MailService, &setting.Mailer{
			SendmailPath:    script,
			SendmailTimeout: time.Minute,
		})()

		err := (&SendmailSender{}).Send("from@example.com", []string{"to@example.com"}, strings.NewReader("body"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit status 69")
		assert.Contains(t, err.Error(), "msmtp: cannot connect to relay")
	})

	t.Run("FailureWithoutStderr", func(t *testing.T) {
		script := writeSendmailScript(t, "cat > /dev/null; exit 75")
		defer test.MockVariableValue(&setting.MailService, &setting.Mailer{
			SendmailPath:    script,
			SendmailTimeout: time.Minute,
		})()

		err := (&SendmailSender{}).Send("from@example.com", []string{"to@example.com"}, strings.NewReader("body"))
		require.Error(t, err)
		assert.Equal(t, "exit status 75", err.Error())
	})

	t.Run("StderrIsBounded", func(t *testing.T) {
		script := writeSendmailScript(t, "cat > /dev/null; head -c 20000 /dev/zero | tr '\\0' x >&2; exit 1")
		defer test.MockVariableValue(&setting.MailService, &setting.Mailer{
			SendmailPath:    script,
			SendmailTimeout: time.Minute,
		})()

		err := (&SendmailSender{}).Send("from@example.com", []string{"to@example.com"}, strings.NewReader("body"))
		require.Error(t, err)
		assert.Equal(t, "exit status 1: "+strings.Repeat("x", sendmailStderrLimit), err.Error())
	})
}

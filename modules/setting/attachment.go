// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import "time"

type AttachmentSettingType struct {
	Storage      *Storage
	AllowedTypes string
	MaxSize      int64
	MaxFiles     int
	Enabled      bool

	// The article editor uploads before the edit is submitted, so any signed-in reader can store
	// files that no commit ever claims. These bound such pending uploads per uploader; zero
	// disables the respective limit.
	ArticleMaxPendingFiles  int64
	ArticleMaxPendingSize   int64 // in MB
	ArticleUploadRateLimit  int64
	ArticleUploadRateWindow time.Duration
}

var Attachment AttachmentSettingType

func loadAttachmentFrom(rootCfg ConfigProvider) (err error) {
	Attachment = AttachmentSettingType{
		AllowedTypes: ".avif,.cpuprofile,.csv,.dmg,.dmp,.docx,.fodg,.fodp,.fods,.fodt,.gif,.gz,.jpeg,.jpg,.json,.jsonc,.log,.md,.mov,.mp4,.odf,.odg,.odp,.ods,.odt,.patch,.pdf,.png,.pptx,.svg,.tgz,.txt,.webm,.webp,.xls,.xlsx,.zip",
		MaxSize:      20,
		MaxFiles:     5,
		Enabled:      true,

		ArticleMaxPendingFiles:  20,
		ArticleMaxPendingSize:   100,
		ArticleUploadRateLimit:  10,
		ArticleUploadRateWindow: time.Minute,
	}
	sec, _ := rootCfg.GetSection("attachment")
	if sec == nil {
		Attachment.Storage, err = getStorage(rootCfg, "attachments", "", nil)
		return err
	}

	Attachment.AllowedTypes = sec.Key("ALLOWED_TYPES").MustString(Attachment.AllowedTypes)
	Attachment.MaxSize = sec.Key("MAX_SIZE").MustInt64(Attachment.MaxSize)
	Attachment.MaxFiles = sec.Key("MAX_FILES").MustInt(Attachment.MaxFiles)
	Attachment.Enabled = sec.Key("ENABLED").MustBool(Attachment.Enabled)
	Attachment.ArticleMaxPendingFiles = sec.Key("ARTICLE_MAX_PENDING_FILES").MustInt64(Attachment.ArticleMaxPendingFiles)
	Attachment.ArticleMaxPendingSize = sec.Key("ARTICLE_MAX_PENDING_SIZE").MustInt64(Attachment.ArticleMaxPendingSize)
	Attachment.ArticleUploadRateLimit = sec.Key("ARTICLE_UPLOAD_RATE_LIMIT").MustInt64(Attachment.ArticleUploadRateLimit)
	Attachment.ArticleUploadRateWindow = sec.Key("ARTICLE_UPLOAD_RATE_WINDOW").MustDuration(Attachment.ArticleUploadRateWindow)
	Attachment.Storage, err = getStorage(rootCfg, "attachments", "", sec)
	return err
}

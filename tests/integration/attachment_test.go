// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

func generateImg() bytes.Buffer {
	// Generate image
	myImage := image.NewRGBA(image.Rect(0, 0, 32, 32))
	var buff bytes.Buffer
	png.Encode(&buff, myImage)
	return buff
}

func uploadAttachmentTo(t *testing.T, session *TestSession, csrf, url, filename string, buff bytes.Buffer, expectedStatus int) string {
	body := &bytes.Buffer{}

	// Setup multi-part
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	assert.NoError(t, err)
	_, err = io.Copy(part, &buff)
	assert.NoError(t, err)
	err = writer.Close()
	assert.NoError(t, err)

	req := NewRequestWithBody(t, "POST", url, body)
	req.Header.Add("X-Csrf-Token", csrf)
	req.Header.Add("Content-Type", writer.FormDataContentType())
	resp := session.MakeRequest(t, req, expectedStatus)

	if expectedStatus != http.StatusOK {
		return ""
	}
	var obj map[string]string
	DecodeJSON(t, resp, &obj)
	return obj["uuid"]
}

func createAttachment(t *testing.T, session *TestSession, csrf, repoURL, filename string, buff bytes.Buffer, expectedStatus int) string {
	return uploadAttachmentTo(t, session, csrf, repoURL+"/issues/attachments", filename, buff, expectedStatus)
}

func createEditorAttachment(t *testing.T, session *TestSession, csrf, repoURL, filename string, buff bytes.Buffer, expectedStatus int) string {
	return uploadAttachmentTo(t, session, csrf, repoURL+"/editor-attachments", filename, buff, expectedStatus)
}

// TestEditorAttachmentServedToRepoReaders verifies that an attachment uploaded via the
// article/file editor (linked only by RepoID, never to an issue/release) is served to anyone
// with repo read permission — not just the uploader — so embedded article images are visible
// to readers, while remaining hidden from users without read access to a private repo.
func TestEditorAttachmentServedToRepoReaders(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := loginUser(t, "user2")
	user8 := loginUser(t, "user8")

	// A fresh request per call: session.MakeRequest stamps the session cookie onto the
	// request, so reusing one *http.Request across sessions would leak the first cookie.
	attachReq := func(uuid string) *RequestWrapper { return NewRequest(t, "GET", "/attachments/"+uuid) }

	// Public repo: readable by the uploader and by any other reader (incl. anonymous). Serving
	// to non-uploaders is the new behavior — previously an unlinked attachment was uploader-only.
	pubUUID := createEditorAttachment(t, owner, GetUserCSRFToken(t, owner), "user2/repo1", "image.png", generateImg(), http.StatusOK)
	owner.MakeRequest(t, attachReq(pubUUID), http.StatusOK)
	user8.MakeRequest(t, attachReq(pubUUID), http.StatusOK)
	MakeRequest(t, attachReq(pubUUID), http.StatusOK) // anonymous

	// Private repo: readable by the owner, blocked for users without read access.
	privUUID := createEditorAttachment(t, owner, GetUserCSRFToken(t, owner), "user2/repo2", "image.png", generateImg(), http.StatusOK)
	owner.MakeRequest(t, attachReq(privUUID), http.StatusOK)
	user8.MakeRequest(t, attachReq(privUUID), http.StatusNotFound)
	MakeRequest(t, attachReq(privUUID), http.StatusNotFound) // anonymous
}

func TestCreateAnonymousAttachment(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	session := emptyTestSession(t)
	createAttachment(t, session, GetAnonymousCSRFToken(t, session), "user2/repo1", "image.png", generateImg(), http.StatusSeeOther)
}

func TestCreateIssueAttachment(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	const repoURL = "user2/repo1"
	session := loginUser(t, "user2")
	uuid := createAttachment(t, session, GetUserCSRFToken(t, session), repoURL, "image.png", generateImg(), http.StatusOK)

	req := NewRequest(t, "GET", repoURL+"/issues/new")
	resp := session.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)

	link, exists := htmlDoc.doc.Find("form#new-issue").Attr("action")
	assert.True(t, exists, "The template has changed")

	postData := map[string]string{
		"_csrf":   htmlDoc.GetCSRF(),
		"title":   "New Issue With Attachment",
		"content": "some content",
		"files":   uuid,
	}

	req = NewRequestWithValues(t, "POST", link, postData)
	resp = session.MakeRequest(t, req, http.StatusOK)
	test.RedirectURL(resp) // check that redirect URL exists

	// Validate that attachment is available
	req = NewRequest(t, "GET", "/attachments/"+uuid)
	session.MakeRequest(t, req, http.StatusOK)

	// anonymous visit should be allowed because user2/repo1 is a public repository
	MakeRequest(t, req, http.StatusOK)
}

func TestGetAttachment(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminSession := loginUser(t, "user1")
	user2Session := loginUser(t, "user2")
	user8Session := loginUser(t, "user8")
	emptySession := emptyTestSession(t)
	testCases := []struct {
		name       string
		uuid       string
		createFile bool
		session    *TestSession
		want       int
	}{
		{"LinkedIssueUUID", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", true, user2Session, http.StatusOK},
		{"LinkedCommentUUID", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a17", true, user2Session, http.StatusOK},
		{"linked_release_uuid", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a19", true, user2Session, http.StatusOK},
		{"NotExistingUUID", "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a18", false, user2Session, http.StatusNotFound},
		{"FileMissing", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a18", false, user2Session, http.StatusInternalServerError},
		{"NotLinked", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a20", true, user2Session, http.StatusNotFound},
		{"NotLinkedAccessibleByUploader", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a20", true, user8Session, http.StatusOK},
		{"PublicByNonLogged", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", true, emptySession, http.StatusOK},
		{"PrivateByNonLogged", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", true, emptySession, http.StatusNotFound},
		{"PrivateAccessibleByAdmin", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", true, adminSession, http.StatusOK},
		{"PrivateAccessibleByUser", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", true, user2Session, http.StatusOK},
		{"RepoNotAccessibleByUser", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", true, user8Session, http.StatusNotFound},
		{"OrgNotAccessibleByUser", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a21", true, user8Session, http.StatusNotFound},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Write empty file to be available for response
			if tc.createFile {
				_, err := storage.Attachments.Save(repo_model.AttachmentRelativePath(tc.uuid), strings.NewReader("hello world"), -1)
				assert.NoError(t, err)
			}
			// Actual test
			req := NewRequest(t, "GET", "/attachments/"+tc.uuid)
			tc.session.MakeRequest(t, req, tc.want)
		})
	}
}

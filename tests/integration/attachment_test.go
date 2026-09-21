// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/queue"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/tests"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notExistingAttachmentUUID is a well-formed UUID that no attachment fixture uses.
const notExistingAttachmentUUID = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a18"

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

// thumbnailAlts returns the alt attributes of the images rendered by the attachment list, i.e. the
// names of the attachments the template decided are not already embedded in the content.
func thumbnailAlts(t *testing.T, attachmentsHTML string) []string {
	t.Helper()
	alts := []string{}
	NewHTMLParser(t, bytes.NewBufferString(attachmentsHTML)).Find(".thumbnails img").Each(func(_ int, s *goquery.Selection) {
		alt, ok := s.Attr("alt")
		assert.True(t, ok, "every thumbnail should carry an alt attribute")
		alts = append(alts, alt)
	})
	return alts
}

// contentVersion reads the current content version of an editable zone (an issue body or a comment)
// off the issue page, so the tests do not have to assume the initial version. updateURLSuffix is
// matched against the zone's data-update-url, which is built from the repository link and may
// therefore carry a different prefix than the URL the test posts to.
func contentVersion(t *testing.T, session *TestSession, pageURL, updateURLSuffix string) string {
	t.Helper()
	resp := session.MakeRequest(t, NewRequest(t, "GET", pageURL), http.StatusOK)
	zone := NewHTMLParser(t, resp.Body).Find(`.edit-content-zone[data-update-url$="` + updateURLSuffix + `"]`)
	version, ok := zone.Attr("data-content-version")
	assert.True(t, ok, "the edit zone of %s should carry a content version", updateURLSuffix)
	return version
}

// TestInlineEditHidesEmbeddedAttachments covers the attachment list returned by the issue and
// comment inline-edit endpoints. The attachments template hides attachments whose UUID already
// appears in the *rendered* content (they are displayed inline by the content itself), which only
// works if the handler passes the rendered content under the "RenderedContent" key. It used to pass
// the raw markdown under "Content", so the filter never applied and every embedded image was listed
// again until the page was reloaded.
func TestInlineEditHidesEmbeddedAttachments(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const repoURL = "/user2/repo1"
	session := loginUser(t, "user2")
	csrf := GetUserCSRFToken(t, session)

	embedded := createAttachment(t, session, csrf, repoURL, "embedded.png", generateImg(), http.StatusOK)
	standalone := createAttachment(t, session, csrf, repoURL, "standalone.png", generateImg(), http.StatusOK)

	// create an issue owning both attachments
	resp := session.MakeRequest(t, NewRequest(t, "GET", repoURL+"/issues/new"), http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)
	link, exists := htmlDoc.doc.Find("form#new-issue").Attr("action")
	assert.True(t, exists, "The template has changed")

	values := url.Values{}
	values.Set("_csrf", htmlDoc.GetCSRF())
	values.Set("title", "issue with an embedded attachment")
	values.Set("content", "initial")
	values.Add("files", embedded)
	values.Add("files", standalone)
	resp = session.MakeRequest(t, NewRequestWithURLValues(t, "POST", link, values), http.StatusOK)
	issueURL := test.RedirectURL(resp)
	_, issueIndex, found := strings.Cut(issueURL, "/issues/")
	assert.True(t, found, "the issue redirect should point at the new issue")
	// the redirect target is the subject-scoped URL, which has no page route of its own; the edit
	// zones (and with them the current content versions) come from the repository-scoped page
	issuePageURL := repoURL + "/issues/" + issueIndex

	var obj struct {
		Content     string `json:"content"`
		Attachments string `json:"attachments"`
	}

	t.Run("issue", func(t *testing.T) {
		// inline-edit the body so that one of the two attachments is embedded in it
		updateURL := issueURL + "/content"
		values := url.Values{}
		values.Set("_csrf", csrf)
		values.Set("content", "![embedded.png](/attachments/"+embedded+")")
		values.Set("content_version", contentVersion(t, session, issuePageURL, "/issues/"+issueIndex+"/content"))
		values.Add("files[]", embedded)
		values.Add("files[]", standalone)
		resp := session.MakeRequest(t, NewRequestWithURLValues(t, "POST", updateURL, values), http.StatusOK)
		DecodeJSON(t, resp, &obj)

		assert.Contains(t, obj.Content, embedded, "the rendered content should embed the attachment")
		assert.Equal(t, []string{"standalone.png"}, thumbnailAlts(t, obj.Attachments),
			"only the attachment that is not embedded in the content should be listed")
	})

	t.Run("comment", func(t *testing.T) {
		// same story for a comment: create one owning both attachments, then inline-edit it
		commentEmbedded := createAttachment(t, session, csrf, repoURL, "comment-embedded.png", generateImg(), http.StatusOK)
		commentStandalone := createAttachment(t, session, csrf, repoURL, "comment-standalone.png", generateImg(), http.StatusOK)

		values := url.Values{}
		values.Set("_csrf", csrf)
		values.Set("content", "initial comment")
		values.Add("files", commentEmbedded)
		values.Add("files", commentStandalone)
		resp := session.MakeRequest(t, NewRequestWithURLValues(t, "POST", issueURL+"/comments", values), http.StatusOK)
		_, commentAnchor, ok := strings.Cut(test.RedirectURL(resp), "#issuecomment-")
		assert.True(t, ok, "the comment redirect should point at the new comment")

		updateURL := repoURL + "/comments/" + commentAnchor
		values = url.Values{}
		values.Set("_csrf", csrf)
		values.Set("content", "![comment-embedded.png](/attachments/"+commentEmbedded+")")
		values.Set("content_version", contentVersion(t, session, issuePageURL, "/comments/"+commentAnchor))
		values.Add("files[]", commentEmbedded)
		values.Add("files[]", commentStandalone)
		resp = session.MakeRequest(t, NewRequestWithURLValues(t, "POST", updateURL, values), http.StatusOK)
		DecodeJSON(t, resp, &obj)

		assert.Contains(t, obj.Content, commentEmbedded, "the rendered comment should embed the attachment")
		assert.Equal(t, []string{"comment-standalone.png"}, thumbnailAlts(t, obj.Attachments),
			"only the attachment that is not embedded in the comment should be listed")
	})
}

// TestArticleAttachmentRouteServesEmbeddedAttachment covers the URL that images embedded in a
// comment actually resolve to. The editor writes "![name](/attachments/{uuid})" and the markup
// renderer resolves that against Repository.Link(), which in Forkana is
// "/article/{owner}/{subject}" — so the attachment must be served from
// "/article/{owner}/{subject}/attachments/{uuid}", otherwise every embedded image 404s.
func TestArticleAttachmentRouteServesEmbeddedAttachment(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // public repo owned by user2
	subjectName := repo1.GetSubject(t.Context())

	// attachment fixture linked to a comment on user2/repo1
	const uuid = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a17"
	_, err := storage.Attachments.Save(repo_model.AttachmentRelativePath(uuid), strings.NewReader("hello world"), -1)
	assert.NoError(t, err)

	articleURL := fmt.Sprintf("/article/%s/%s/attachments/%s", repo1.OwnerName, subjectName, uuid)

	// A fresh request per call: MakeRequest stamps session cookies onto the request.
	session := loginUser(t, "user2")
	session.MakeRequest(t, NewRequest(t, "GET", articleURL), http.StatusOK)
	MakeRequest(t, NewRequest(t, "GET", articleURL), http.StatusOK) // anonymous, repo is public

	// unknown attachments still 404 instead of leaking anything
	MakeRequest(t, NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/attachments/%s", repo1.OwnerName, subjectName, notExistingAttachmentUUID)), http.StatusNotFound)

	// The route carries no repository middleware, so permission is entirely ServeAttachment's
	// job: it resolves the attachment's own repository. An attachment on a private repository
	// must stay hidden from anonymous visitors through this URL too.
	// repo2 has no subject, so Link() falls back to the repo name — the URL still has to work,
	// which is another reason the route resolves no repository of its own.
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2}) // private repo owned by user2
	const privUUID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"
	_, err = storage.Attachments.Save(repo_model.AttachmentRelativePath(privUUID), strings.NewReader("hello world"), -1)
	assert.NoError(t, err)

	privURL := fmt.Sprintf("/article/%s/%s/attachments/%s", repo2.OwnerName, repo2.GetSubject(t.Context()), privUUID)
	session.MakeRequest(t, NewRequest(t, "GET", privURL), http.StatusOK)
	MakeRequest(t, NewRequest(t, "GET", privURL), http.StatusNotFound)                       // anonymous
	loginUser(t, "user8").MakeRequest(t, NewRequest(t, "GET", privURL), http.StatusNotFound) // no read access
}

// TestArticleAttachmentListingRoutes covers the attachment listing URLs the edit-in-place
// dropzone requests. It builds them from $.RepoLink, which is the article link, so the listing
// has to be served under "/article/{owner}/{subject}" as well — a 404 there leaves the dropzone
// empty and the following save submits an empty "files[]", deleting every attachment.
func TestArticleAttachmentListingRoutes(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // public repo owned by user2
	articleBase := fmt.Sprintf("/article/%s/%s", repo1.OwnerName, repo1.GetSubject(t.Context()))

	session := loginUser(t, "user2")
	// issue 1 on repo1, and comment 2 which carries attachment ...a17
	session.MakeRequest(t, NewRequest(t, "GET", articleBase+"/issues/1/attachments"), http.StatusOK)
	resp := session.MakeRequest(t, NewRequest(t, "GET", articleBase+"/comments/2/attachments"), http.StatusOK)
	assert.Contains(t, resp.Body.String(), "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a17")
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

// TestEditorAttachmentRecordsArticlePurpose verifies that the editor upload endpoint records the
// article purpose while the issue endpoint keeps the unspecified one, which is what keeps the
// article-attachment garbage collector away from unfinished issue and release drafts.
func TestEditorAttachmentRecordsArticlePurpose(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")
	csrf := GetUserCSRFToken(t, session)

	editorUUID := createEditorAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
	editorAttach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: editorUUID})
	assert.Equal(t, repo_model.AttachmentPurposeArticle, editorAttach.Purpose)

	issueUUID := createAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
	issueAttach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: issueUUID})
	assert.Equal(t, repo_model.AttachmentPurposeUnspecified, issueAttach.Purpose)
}

// TestDeleteAttachmentRefusesAssociated covers the shared-lifetime rule: once an attachment is
// referenced by committed article content, its uploader may no longer remove it directly, because
// other repositories — forks included — reference the same blob. A pending upload stays removable.
func TestDeleteAttachmentRefusesAssociated(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const removeURL = "user2/repo1/issues/attachments/remove"
	session := loginUser(t, "user2")
	csrf := GetUserCSRFToken(t, session)

	removeReq := func(uuid string) *RequestWrapper {
		return NewRequestWithValues(t, "POST", removeURL, map[string]string{"_csrf": csrf, "file": uuid})
	}

	t.Run("PendingUploadIsRemovable", func(t *testing.T) {
		uuid := createEditorAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
		session.MakeRequest(t, removeReq(uuid), http.StatusOK)
		unittest.AssertNotExistsBean(t, &repo_model.Attachment{UUID: uuid})
	})

	t.Run("AssociatedAttachmentIsRefused", func(t *testing.T) {
		uuid := createEditorAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
		attach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: uuid})
		require.NoError(t, repo_model.AddArticleAttachments(t.Context(), attach.RepoID, []int64{attach.ID}))

		session.MakeRequest(t, removeReq(uuid), http.StatusConflict)
		unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: uuid})
	})

	// A linked issue attachment carries no association, so its removal path is unchanged.
	t.Run("IssueAttachmentIsUnaffected", func(t *testing.T) {
		uuid := createAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
		session.MakeRequest(t, removeReq(uuid), http.StatusOK)
		unittest.AssertNotExistsBean(t, &repo_model.Attachment{UUID: uuid})
	})
}

// TestArticleCommitAssociatesAttachments exercises the web/API commit hook end to end: the
// association is derived from the blob as it was actually committed, so it only exists once the
// ref has moved. It needs a running instance because the commit goes through the push hooks.
func TestArticleCommitAssociatesAttachments(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, _ *url.URL) {
		session := loginUser(t, "user2")
		csrf := GetUserCSRFToken(t, session)
		user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		articleContent := func(uuid string) string {
			return fmt.Sprintf("# Article\n\n![img](/attachments/%s)\n", uuid)
		}
		associated := func(t *testing.T, repoID, attachmentID int64) bool {
			t.Helper()
			has, err := repo_model.HasArticleAttachment(t.Context(), repoID, attachmentID)
			require.NoError(t, err)
			return has
		}

		t.Run("ReferenceFromUploadRepo", func(t *testing.T) {
			uuid := createEditorAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
			attach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: uuid})

			_, err := createFileInBranch(user2, repo1, "article-associated.md", repo1.DefaultBranch, articleContent(uuid))
			require.NoError(t, err)
			assert.True(t, associated(t, repo1.ID, attach.ID))
		})

		t.Run("CommitWithoutReferenceAssociatesNothing", func(t *testing.T) {
			uuid := createEditorAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
			attach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: uuid})

			_, err := createFileInBranch(user2, repo1, "article-plain.md", repo1.DefaultBranch, "# Article\n")
			require.NoError(t, err)
			assert.False(t, associated(t, repo1.ID, attach.ID))
		})

		// Attachment 2 belongs to another repository and to an issue there. Referencing it must
		// neither fail the commit nor grant repo1's readers access.
		t.Run("UnauthorizedReferenceIsSkipped", func(t *testing.T) {
			attach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: 2})

			_, err := createFileInBranch(user2, repo1, "article-borrowed.md", repo1.DefaultBranch, articleContent(attach.UUID))
			require.NoError(t, err)
			assert.False(t, associated(t, repo1.ID, attach.ID))
		})
	})
}

// TestArticlePushAssociatesAttachments covers the central post-ref-update hook: article content
// that arrives by plain Git push, without ever passing through the editor, is discovered too.
func TestArticlePushAssociatesAttachments(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		session := loginUser(t, "user2")
		csrf := GetUserCSRFToken(t, session)
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		uuid := createEditorAttachment(t, session, csrf, "user2/repo1", "image.png", generateImg(), http.StatusOK)
		attach := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{UUID: uuid})

		oldPath, oldUser := u.Path, u.User
		defer func() { u.Path, u.User = oldPath, oldUser }()
		u.Path = repo1.FullName() + ".git"
		u.User = url.UserPassword("user2", userPassword)

		dstPath := t.TempDir()
		doGitClone(dstPath, u)(t)

		content := fmt.Sprintf("# Article\n\n![img](/attachments/%s)\n", uuid)
		require.NoError(t, os.WriteFile(filepath.Join(dstPath, "README.md"), []byte(content), 0o644))
		require.NoError(t, git.AddChanges(t.Context(), dstPath, true))
		signature := git.Signature{Email: "user2@example.com", Name: "user2"}
		require.NoError(t, git.CommitChanges(t.Context(), dstPath, git.CommitChangesOptions{
			Committer: &signature,
			Author:    &signature,
			Message:   "embed an attachment",
		}))
		doGitPushTestRepository(dstPath, "origin", "master")(t)

		// pushUpdates runs on the push queue, so the association is not visible synchronously.
		require.NoError(t, queue.GetManager().FlushAll(t.Context(), 30*time.Second))

		has, err := repo_model.HasArticleAttachment(t.Context(), repo1.ID, attach.ID)
		require.NoError(t, err)
		assert.True(t, has)
	})
}

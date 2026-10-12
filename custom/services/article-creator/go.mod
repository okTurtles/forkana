module github.com/okTurtles/forkana/custom/services/article-creator

go 1.25.1

require code.gitea.io/gitea v0.0.0-00010101000000-000000000000

require golang.org/x/text v0.39.0 // indirect

// The subject title rule lives in the server (modules/subjecttitle) and is shared from this
// checkout, so that article-creator cleans titles exactly like the server validates them.
replace code.gitea.io/gitea => ../../..

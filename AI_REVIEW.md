# AI Review

Here's how to review PRs using AI.

Most of the time you don't need to do this by hand: comment `/review` on the PR and
the [`pull-review-crush.yml`](.github/workflows/pull-review-crush.yml) workflow runs
the same review automatically. This document is for the manual flow (pasting a diff
into a chat with a SOTA model), and the template below mirrors the prompt built by
that workflow's "Build review prompt" step. **If you change one, change the other.**

## Template

````
Attached are a set of changes to a Go project that uses Vuejs. It is based on Gitea and has been modified into a type of online encyclopedia, where each subject can have multiple articles, and each article is represented by a git repository with a primary markdown file that contains the article. Articles can be forked, forming a tree of articles under the subject.

Please have a look at these changes and thoroughly check them for:

- Highest priority: any bugs, security issues, DRY violations, and improvements that can be made through code simplification
- spelling or grammar mistakes in user-facing strings or log messages
- poorly named identifiers like variables, selectors, functions/methods, types or classes

When providing feedback, be specific and quote the concrete lines that are problematic, and then give your code improvement suggestions complete with actual code. When replying, always use Markdown and Markdown code fences to quote any lines. Avoid commenting on code that has no issues (no "you did a good job here!" fluff).

When referencing sections of code, always mention the specific lines being referenced in the following manner to make it easy for developers to know where to look:

- `file.js:841`
- `file.vue:50-70`

For each issue you identify, give it a rating of 🔴 for high importance & high confidence, 🟡 for medium importance & high confidence, and ⚪️ for issues of either lower confidence or lower importance. Prioritize finding 🔴 and 🟡 issues. Give your feedback in order of importance from most to least and number the issues sequentially in their headings.

Also, for each issue, immediately after the issue heading please add these two checkboxes:

- [ ] Addressed
- [ ] Dismissed

Pay attention to any parts of the project that might break or conflict because of the changes, or any bits that are no longer relevant and should be removed as a result of the changes.

If no problems are found, state that and do nothing else.

Finally, if you have any questions about the changes or need to see any additional context or files to help with your review please let me know and I'll supply it.

### Helpful Context

### Changes to Review

```diff

```
````

## Checking out the PR

Then:

1. `git pull` the `master` branch to make sure you have the latest changes from `master`
2. Check out the PR you want to review in a local branch.
3. Run `git merge master` so that there is an absolute minimal set of changes when running the next command
4. Run the diff command below

```sh
git diff master -U10 \
  -- . \
  ':!package-lock.json' ':!**/package-lock.json' \
  ':!pnpm-lock.yaml' ':!**/pnpm-lock.yaml' \
  ':!yarn.lock' ':!**/yarn.lock' \
  ':!go.sum' ':!**/go.sum' \
  ':!*.min.js' ':!*.min.css' \
  ':!node_modules/**' ':!vendor/**' \
  ':!dist/**' ':!build/**' \
  ':!*.svg' ':!*.png' ':!*.jpg' ':!*.jpeg' ':!*.gif' ':!*.ico' ':!*.webp' \
  ':!*.woff' ':!*.woff2' ':!*.ttf' ':!*.eot' \
  | pbcopy
```

The pathspec exclusions match the workflow's "Generate diff" step, so lockfiles,
minified bundles, vendored code and binary assets stay out of the prompt.

The last part (`pbcopy`) is a command on macOS to copy the output of the prior command to the clipboard. If you don't use macOS you can omit this part or use the equivalent on your system (e.g. `wl-copy` on Fedora).

## Update the Template

- Paste the resulting diff inside of the code fences in the last section of template above.
- Remove any remaining excess information the exclusions above didn't catch.
- Either remove the "Helpful Context" section or add additional helpful context there. Examples of "helpful context" include: a description of what the changes are about, the full contents of various other relevant files, etc.

Then send this to the latest state-of-the-art (SOTA) LLM.

If the model you're using can read the repository itself (an agentic CLI rather than a
chat window), also tell it that it has full access to the source code and should use
subagents when investigating specific questions ("where is this function called?") to
conserve tokens — and that it must not modify any files.

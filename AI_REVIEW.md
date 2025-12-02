# AI Review

Here's how to review PRs using AI.

## Template

````
Attached are a set of changes to a Go project that uses Vuejs.

Please have a look at these changes and thoroughly check it for any bugs or security issues. Also check if anything could be improved. When providing feedback, be specific and quote the concrete lines that are problematic, and then give your code improvement suggestions complete with actual code. When replying, always use Markdown and Markdown code fences to quote any lines. Avoid commenting on code that has nothing wrong with it.

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
4. Run `git diff master -U15 | pbcopy`

The last part (`pbcopy`) is a command on macOS to copy the output of the prior command to the clipboard. If you don't use macOS you can omit this part or use the equivalent on your system (e.g. `wl-copy` on Fedora).

## Update the Template

- Paste the resulting diff inside of the code fences in the last section of template above.
- Remove any unnecessary excess information (like `package-lock.json` or `deno.lock` changes)
- Either remove the "Helpful Context" section or add additional helpful context there. Examples of "helpful context" include: a description of what the changes are about, the full contents of various other relevant files, etc.

Then send this to the latest state-of-the-art (SOTA) LLM.

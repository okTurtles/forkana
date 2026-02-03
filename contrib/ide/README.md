# IDE and code editor configuration

## Table of Contents
- [IDE and code editor configuration](#ide-and-code-editor-configuration)
  - [Microsoft Visual Studio Code](#microsoft-visual-studio-code)
  - [Remote Delve Debugging](#remote-delve-debugging)

## Microsoft Visual Studio Code
Download Microsoft Visual Studio Code at https://code.visualstudio.com/ and follow instructions at https://code.visualstudio.com/docs/languages/go to setup Go extension for it.

Create new directory `.vscode` in Gitea root folder and copy contents of folder [contrib/ide/vscode](vscode/) to it. You can now use `Ctrl`+`Shift`+`B` to build gitea executable and `F5` to run it in debug mode.

Supported on Debian, Ubuntu, Red Hat, Fedora, SUSE Linux, MacOS and Microsoft Windows.

## Remote Delve Debugging
For advanced debugging with breakpoints and variable inspection using external IDEs (GoLand, VSCode, etc.), see [Delve Debugging](DELVE_DEBUGGING.md). This guide explains how to run the backend through Delve while using Air for hot-reloading, allowing you to attach your IDE's debugger to a live development server.

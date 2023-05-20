# Contributing to YugabyteDB Anywhere Terraform Provider

This document outlines the basic steps required to work with and contribute to the YugabyteDB Anywhere Terraform Provider codebase.

## Talk to us

Before doing any code changes,
it's a good idea to reach out to us,
so as to make sure there's a general consencus on the proposed change and the implementation strategy.
You can reach us by creating a GitHub Issue [here](https://github.com/yugabyte/terraform-provider-yugabytedb-anywhere/issues)

## Install the tools

The following software is required to work with the YugabyteDB Anywhere Terraform Provider codebase and build it locally:

* [Terraform CLI](https://developer.hashicorp.com/terraform/downloads) - v1.4.6 or later
* [Golang](https://go.dev/doc/install) - v1.18.3 or later

See the links above for installation instructions on your platform. You can verify the versions are installed and running:

```sh
    terraform -v
    go version
```

### GitHub account

YugabyteDB Anywhere Terraform Provider uses [GitHub](https://github.com) for its primary code repository and for pull-requests, so if you don't already have a GitHub account you'll need to [join](https://github.com/join).

## Working with the codebase

### Fork the YugabyteDB Anywhere Terraform Provider repository

Go to the [YugabyteDB Anywhere Terraform Provider repository](https://github.com/yugabyte/terraform-provider-yugabytedb-anywhere) and press the "Fork" button near the upper right corner of the page. When finished, you will have your own "fork" at `https://github.com/<your-username>/terraform-provider-yba`, and this is the repository to which you will upload your proposed changes and create pull requests. For details, see the [GitHub documentation](https://help.github.com/articles/fork-a-repo/).

### Clone your fork

At a terminal, go to the directory in which you want to place a local clone of the YugabyteDB Anywhere Terraform Provider repository, and run the following commands to use HTTPS authentication:

```sh
    git clone https://github.com/<your-username>/terraform-provider-yba.git
```

If you prefer to use SSH and have [uploaded your public key to your GitHub account](https://help.github.com/articles/adding-a-new-ssh-key-to-your-github-account/), you can instead use SSH:

```sh
    git clone git@github.com:<your-username>/terraform-provider-yba.git
```

This will create a `terraform-provider-yba` directory, so change into that directory:

```sh
    cd terraform-provider-yba
```

This repository knows about your fork, but it doesn't yet know about the official or ["upstream" YugabyteDB Anywhere Terraform Provider repository](https://github.com/yugabyte/terraform-provider-yugabytedb-anywhere). Run the following commands:

```sh
    git remote add upstream https://github.com/yugabyte/terraform-provider-yba.git
    git fetch upstream
    git branch --set-upstream-to=upstream/main main
```

Now, when you check the status using Git, it will compare your local repository to the *upstream* repository.

### Get the latest upstream code

You will frequently need to get all the of the changes that are made to the upstream repository, and you can do this with these commands:

```sh
    git fetch upstream
    git pull upstream main
```

The first command fetches all changes on all branches, while the second actually updates your local `main` branch with the latest commits from the `upstream` repository.

### Building locally

To build the source code locally, checkout and update the `main` branch:

```sh
    git checkout main
    git pull upstream main
```

Create folder in [implied directory](https://developer.hashicorp.com/terraform/cli/config/config-file#implied-local-mirror-directories) to hold the binary. Directory format:

```sh
    mkdir -p <implied_mirror_directory>/terraform.yugabyte.com/platform/yugabyte-platform/<provider_version>/<system_architecture>
```

Switch to the root directory (`terraform-provider-yba`) of the terraform repo and build the binary with the command:

```sh
    go build -o <implied_mirror_directory>/terraform.yugabyte.com/platform/yugabyte-platform/<provider_version>/<system_architecture>/
```

### Running and debugging tests

Acceptance tests defined in the source code can be used to debug the newly added codebase using the command:

```sh
    make acctest
```

## Making changes

Everything the community does with the codebase -- fixing bugs, adding features, making improvements, adding tests, etc. -- should be described by an issue in the [GitHub issues](https://github.com/yugabyte/terraform-provider-yugabytedb-anywhere/issues) page. If no such issue exists for what you want to do, please create an issue with a meaningful and easy-to-understand description.
If you are going to work on a specific issue and it's your first contribution,
please add a short comment to the issue, so other people know you're working on it.

Before you make any changes, be sure to switch to the `main` branch and pull the latest commits on the `main` branch from the upstream repository. Also, it's probably good to run a build and verify all tests pass *before* you make any changes.

```sh
    git checkout main
    git pull upstream main
    make testacc
```

Once everything builds, create a *topic branch* named appropriately:

```sh
    git checkout -b fix_metadata_yba_cloud_provider
```

This branch exists locally and it is there you should make all of your proposed changes for the issue. As you'll soon see, it will ultimately correspond to a single pull request that the YugabyteDB Anywhere Terraform Provider committers will review and merge (or reject) as a whole. (Some issues are big enough that you may want to make several separate but incremental sets of changes. In that case, you can create subsequent topic branches for the same issue by appending a short suffix to the branch name.)

Your changes should include changes to existing tests or additional accepatnce tests that verify your changes work. We recommend frequently running related acceptance tests (in your IDE) to make sure your changes didn't break anything else, and that you also periodically run a complete build to make sure that everything still works.

Feel free to commit your changes locally as often as you'd like, though we generally prefer that each commit represent a complete and atomic change to the code. Often, this means that most issues will be addressed with a single commit in a single pull-request, but other more complex issues might be better served with a few commits that each make separate but atomic changes. (Some developers prefer to commit frequently and to ammend their first commit with additional changes. Other developers like to make multiple commits and to then squash them. How you do this is up to you. However, *never* change, squash, or ammend a commit that appears in the history of the upstream repository.) When in doubt, use a few separate atomic commits; if the YugabyteDB Anywhere Terraform Provider reviewers think they should be squashed, they'll let you know when they review your pull request.

Committing is as simple as:

```sh
    git commit .
```

which should then pop up an editor of your choice in which you should place a good commit message. **We do expect that all commit messages begin with a line starting with the GitHub Issue and ending with a short phrase that summarizes what changed in the commit.** For example:

```md
    Fixing metadata structuring in yba_cloud_provider
```

If that phrase is not sufficient to explain your changes, then the first line should be followed by a blank line and one or more paragraphs with additional details.

As an exception, commits for trivial documentation changes which don't warrant the creation of an issue can be prefixed with `[docs]`, for example:

```md
    [docs] Typo fix in yba_installation documentation
```

### Code Formatting

This project utilizes a set of code style rules that are automatically applied by the build process.  There are two ways in which you can apply these rules while contributing:

1. Command `terraform fmt` can be used to format terraform configuration files.

2. Command `go fmt` can be used to format go source files.

3. With the command `arc lint` the code style rules are pointed out.

### Rebasing

If its been more than a day or so since you created your topic branch, we recommend *rebasing* your topic branch on the latest `main` branch. This requires switching to the `main` branch, pulling the latest changes, switching back to your topic branch, and rebasing:

```sh
    git checkout main
    git pull upstream main
    git checkout fix_metadata_yba_cloud_provider
    git rebase main
```

If your changes are compatible with the latest changes on `main`, this will complete and there's nothing else to do. However, if your changes affect the same files/lines as other changes have since been merged into the `main` branch, then your changes conflict with the other recent changes on `main`, and you will have to resolve them. The git output will actually tell you you need to do (e.g., fix a particular file, stage the file, and then run `git rebase --continue`), but if you have questions consult Git or GitHub documentation or spend some time reading about Git rebase conflicts on the Internet.

### Documentation

When adding new features such as e.g. a connector or configuration options, they must be documented accordingly in the YugabyteDB Anywhere Terraform Provider.
To generate an outline of the code, run the following command:

```sh
    make documents
```

Any additional information can be supplemented in the generated file.

The same applies when changing existing behaviors, e.g. type mappings, removing options etc.

Any documentation update should be part of the pull request you submit for the code change.

### Creating a pull request

Once you're finished making your changes, your topic branch should have your commit(s) and you should have verified that your branch builds successfully. At this point, you can shared your proposed changes and create a pull request. To do this, first push your topic branch (and its commits) to your fork repository (called `origin`) on GitHub:

```sh
    git push origin fix_metadata_yba_cloud_provider
```

Then, in a browser go to your forked repository, and you should see a small section near the top of the page with a button labeled "Contribute". GitHub recognized that you pushed a new topic branch to your fork of the upstream repository, and it knows you probably want to create a pull request with those changes. Click on the button, and a button "Open pull request" will apper. Click it and GitHub will present you the "Comparing changes" page, where you can view all changes that you are about to submit. With all revised, click in "Create pull request" and a short form will be given, that you should fill out with information about your pull request. The title should start with the GitHub issue and end with a short phrase that summarizes the changes included in the pull request. (If the pull request contains a single commit, GitHub will automatically prepopulate the title and description fields from the commit message.)

When completed, press the "Create" button.

At this point, you can switch to another issue and another topic branch. The YugabyteDB Anywhere Terraform Provider committers will be notified of your new pull request, and will review it in short order. They may ask questions or make remarks using line notes or comments on the pull request. (By default, GitHub will send you an email notification of such changes, although you can control this via your GitHub preferences.)

If the reviewers ask you to make additional changes, simply switch to your topic branch for that pull request:

```sh
    git checkout fix_metadata_yba_cloud_provider
```

and then make the changes on that branch and either add a new commit or ammend your previous commits. When you've addressed the reviewers' concerns, push your changes to your `origin` repository:

```sh
    git push origin fix_metadata_yba_cloud_provider
```

GitHub will automatically update the pull request with your latest changes, but we ask that you go to the pull request and add a comment summarizing what you did. This process may continue until the reviewers are satisfied.

By the way, please don't take offense if the reviewers ask you to make additional changes, even if you think those changes are minor. The reviewers have a broad understanding of the codebase, and their job is to ensure the code remains as uniform as possible, is of sufficient quality, and is thoroughly tested. When they believe your pull request has those attributes, they will merge your pull request into the official upstream repository.

Once your pull request has been merged, feel free to delete your topic branch both in your local repository:

```sh
    git branch -d fix_metadata_yba_cloud_provider
```

and in your fork:

```sh
    git push origin :fix_metadata_yba_cloud_provider
```

(This last command is a bit strange, but it basically is pushing an empty branch (the space before the `:` character) to the named branch. Pushing an empty branch is the same thing as removing it.)

### Continuous Integration

The project currently builds its jobs in one environment:

* [GitHub Actions](https://github.com/yugabyte/terraform-provider-yugabytedb-anywhere/actions) for pull requests:
  * Tests run only against the current version of provider on latest stable release

### Summary

Here's a quick check list for a good pull request (PR):

* Discussed and approved on GitHub Issues
* A GitHub issue associated with your PR
* One feature/change per PR
* New/changed features have been documented
* A full build completes successfully
* Do a rebase on upstream `main`

## PR Handling (For committers)

* No code changes without PR
* Don't merge your own PRs, ensure four eyes principle
* Don't do force pushes to main branch
* Always apply PRs via rebasing instead of merges (for a linear commit history)
* Optional: squash commits into one, if intermediate commits are not relevant in the long run

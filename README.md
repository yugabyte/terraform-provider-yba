# Terraform Provider Yugabyte Anywhere

This Terraform provider manages the following resources for Yugabyte Anywhere:
* Cloud Provider
* Universe
* Backup Storage Configuration
* Backup Schedules
* Users
* Customers

In addition, there are modules included for installing and managing platform instances in the following clouds:
* AWS
* GCP
* Azure

## Requirements

-	[Terraform](https://www.terraform.io/downloads.html) >= 0.13.x
-	[Go](https://golang.org/doc/install) >= 1.15

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command: 
```sh
$ go install
```

Once you've written your provider, you'll want to [publish it on the Terraform Registry](https://www.terraform.io/docs/registry/providers/publishing.html) so that others can use it.

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```sh
$ make testacc
```

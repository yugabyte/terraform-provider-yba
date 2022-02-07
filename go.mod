module github.com/yugabyte/terraform-provider-yugabyte-platform

go 1.15

require (
	github.com/go-logr/zapr v1.1.0
	github.com/go-openapi/strfmt v0.20.3
	github.com/hashicorp/terraform-plugin-docs v0.5.1
	github.com/hashicorp/terraform-plugin-log v0.2.1
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.10.1
	github.com/yugabyte/yb-tools v0.0.0-20220201221806-4560e6b1db86
	go.uber.org/zap v1.21.0
)

replace github.com/juju/persistent-cookiejar v0.0.0-20171026135701-d5e5a8405ef9 => github.com/andrewstuart/persistent-cookiejar v0.0.0-20181121031108-afb54bd74b6b

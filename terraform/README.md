# Notes for happier terraforming with PostmanPat and Github spaces

## Setup

* Remember to setup `gh-codespace-token` (currently called `DO_PAT`) in the [Codespaces Secrets](https://github.com/aaronromeo/postmanpat/settings/secrets/codespaces)
* Run `source terraform/pre.sh`
* Run `terraform plan --var="DO_TOKEN=dop_v1_XXXXXXXXXXXX"` or `terraform plan --var="DO_TOKEN=$DO_PAT"`

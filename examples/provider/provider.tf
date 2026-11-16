terraform {
  required_providers {
    uup = {
      source = "uupdev/uup"
    }
  }
}

# The token is read from UUP_API_TOKEN when api_token is left out.
provider "uup" {}

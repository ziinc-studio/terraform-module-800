terraform {
  required_providers {
    eighthundred = {
      source  = "ziinc-studio/eighthundred"
      version = "~> 0.1"
    }
  }
}

# token is read from the EIGHT_HUNDRED_API_TOKEN environment variable.
# default_company_id is read from EIGHT_HUNDRED_COMPANY_ID if not set here.
provider "eighthundred" {
  default_company_id = 334176
}

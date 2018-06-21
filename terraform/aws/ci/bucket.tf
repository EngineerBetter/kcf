variable "region" {}

provider "aws" {
  region = "${var.region}"
}

resource "aws_s3_bucket" "ci" {
  bucket = "kcf-pipeline"
  acl    = "private"
}

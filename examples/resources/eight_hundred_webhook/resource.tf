resource "eight_hundred_webhook" "sms_received" {
  url      = "https://hooks.example.com/inbound-sms"
  method   = "POST"
  features = ["sms_received"]
}

# Import:
#   terraform import eight_hundred_webhook.sms_received 334176:42

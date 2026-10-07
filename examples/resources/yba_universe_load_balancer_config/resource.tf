# YBA attaches load balancers that already exist in the universe's cloud
# account: on AWS and Azure by load balancer name, on GCP by backend service
# name. This example creates one with the AWS provider and uses the names of
# others that exist already.
resource "aws_lb" "primary" {
  name               = "yb-primary-nlb"
  load_balancer_type = "network"
  internal           = true
  subnets            = var.subnet_ids
}

# One resource holds every load balancer of the universe.
resource "yba_universe_load_balancer_config" "main" {
  universe_uuid = yba_universe.main.id

  # One load balancer for every availability zone of us-west-2 in the primary
  # cluster.
  load_balancer {
    region  = "us-west-2"
    lb_name = aws_lb.primary.name
  }

  # A load balancer for most zones of us-east-1, and a separate one for
  # us-east-1c.
  load_balancer {
    region  = "us-east-1"
    lb_name = "yb-east-nlb"
    az_overrides = {
      "us-east-1c" = "yb-east-1c-nlb"
    }
  }

  # The load balancer of the read replica cluster.
  load_balancer {
    region       = "us-west-2"
    lb_name      = "yb-read-replica-nlb"
    read_replica = true
  }

  timeouts {
    create = "1h"
    update = "1h"
    delete = "1h"
  }
}

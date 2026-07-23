# Attach externally-created load balancers to a universe. The load balancers
# (AWS/Azure: load balancer name, GCP: backend service name) must already
# exist in the universe's cloud account — for example created with the same
# Terraform configuration through the cloud's own provider.

resource "aws_lb" "primary" {
  name               = "yb-primary-nlb"
  load_balancer_type = "network"
  internal           = true
  subnets            = var.subnet_ids
}

resource "yba_universe_load_balancer_config" "main" {
  universe_uuid = yba_universe.main.id

  # Region-wide load balancer: applied to every AZ of us-west-2 in the
  # primary cluster.
  load_balancer {
    region  = "us-west-2"
    lb_name = aws_lb.primary.name
    lb_fqdn = aws_lb.primary.dns_name
  }

  # Zone-local load balancing: az_overrides points individual AZs at their
  # own load balancer instead of the region default.
  load_balancer {
    region  = "us-east-1"
    lb_name = aws_lb.east.name
    az_overrides = {
      "us-east-1c" = aws_lb.east_zonal.name
    }
  }

  # Read replica traffic through its own load balancer (the universe's
  # ASYNC cluster; YBA supports at most one read replica per universe).
  load_balancer {
    region       = "us-west-2"
    lb_name      = aws_lb.read_replica.name
    read_replica = true
  }

  timeouts {
    create = "1h"
    update = "1h"
    delete = "1h"
  }
}

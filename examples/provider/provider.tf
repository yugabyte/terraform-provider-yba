provider "yb" {
    // unauthenticated - to use provider for installation and customer creation  
    alias = "unauthenticated"
    host = "<host ip address>:80"
}

provider "yb" {
    // after customer creation, use authenticated provider
    host      = "<host ip address>:80"
    api_token = "<customer api token>"
}
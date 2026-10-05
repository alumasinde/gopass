# Testing

## Unit/API
Run `go test ./...` after dependencies are available. The suite should include service, handler, authorization, tenant-isolation and workflow integration tests.

## MySQL integration
Use a dedicated database and DSN; never point tests at development/production data. Example:

```powershell
$env:DB_DSN='testuser:testpass@tcp(127.0.0.1:3306)/gopass_test?parseTime=true&charset=utf8mb4'
go test ./... -count=1
```

## Required authorization cases
- Tenant A cannot read Tenant B resources.
- Security Officer cannot manage roles/users.
- Receptionist cannot perform security overrides.
- Organization Administrator cannot access platform-only operations.
- Site-scoped users cannot read resources outside their site.
- Gate-scoped users cannot operate another gate.

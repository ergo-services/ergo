---
description: TLS Certificate Management
---

# CertManager

Network communication in production systems needs encryption. TLS provides this, but managing TLS certificates introduces operational challenges. Certificates expire. Security incidents require rotation. Updating certificates traditionally means restarting services, causing downtime.

The naive approach loads certificates at startup from files. When you need to update a certificate, you replace the file and restart the service. For a single service, this works. For distributed systems with dozens of nodes and services, coordinating restarts for certificate updates becomes an operational burden.

Ergo Framework provides `gen.CertManager` for live certificate updates. Load a certificate at startup, and you can update it later without restarting. All components using that certificate manager - node acceptors, web servers, TCP servers - automatically use the updated certificate for new connections.

## Creating a Certificate Manager

Create a certificate manager with an initial certificate:

```go
cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
if err != nil {
    panic(err)
}

certManager := gen.CreateCertManager(cert)
```

For development or testing, generate a self-signed certificate:

```go
cert, err := lib.GenerateSelfSignedCert("MyService v1.0")
if err != nil {
    panic(err)
}

certManager := gen.CreateCertManager(cert)
```

Note: Self-signed certificates require setting `InsecureSkipVerify: true` in network options to bypass certificate validation. This is acceptable for development but never use it in production.

Pass the certificate manager to node options:

```go
options.CertManager = certManager
node, err := ergo.StartNode("node@localhost", options)
```

The node's network stack uses this certificate manager for TLS connections. Acceptors use it for incoming connections. Outgoing connections use it for client certificates if needed.

## Updating Certificates

Update the certificate while the node is running:

```go
newCert, err := tls.LoadX509KeyPair("new_cert.pem", "new_key.pem")
if err != nil {
    return err
}

certManager.Update(newCert)
```

The update takes effect immediately for new connections. Existing connections continue using the old certificate until they close. This allows graceful rotation - new connections get the new certificate, old connections finish naturally.

Components using the certificate manager obtain certificates through `GetCertificate` or `GetCertificateFunc`. These methods return the current certificate, so updates automatically propagate to all users of the manager.

## Certificate Lifecycle

The typical pattern involves periodic certificate renewal. A cron job or external process watches for approaching expiration. When renewal is needed, it obtains a new certificate (from Let's Encrypt, an internal CA, or however your infrastructure manages certificates) and calls `Update` on the certificate manager.

The certificate manager is passive - it doesn't handle renewal itself. It provides the mechanism for live updates. Your renewal logic is external, allowing integration with whatever certificate provisioning system you use.

This separation is intentional. Certificate renewal policies vary widely. Some organizations use Let's Encrypt with automated renewal. Others use internal CAs with manual processes. Some rotate certificates on a schedule, others only when necessary. The certificate manager doesn't impose policy - it just enables live updates however you choose to implement them.

For complete certificate manager methods and usage, refer to the `gen.CertManager` interface documentation in the code.

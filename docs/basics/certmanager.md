---
description: TLS Certificate Management
---

# CertManager

`gen.CertManager` stores the certificate used by TLS listeners and clients. Replace that certificate with `Update` while the node is running; new handshakes read the current value.

The manager does not obtain or renew certificates. Your renewal service supplies them. Existing connections retain the TLS session established before an update.

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

## Mutual TLS

For scenarios requiring client certificate authentication, use `gen.CertAuthManager`. It extends `CertManager` with CA pool management for verifying certificates on both sides of the connection. This enables mutual TLS (mTLS) where servers verify client certificates and clients verify server certificates.

```go
certManager := gen.CreateCertAuthManager(cert)
certManager.SetClientCAs(clientCAPool)    // server verifies clients
certManager.SetRootCAs(serverCAPool)      // client verifies servers
certManager.SetClientAuth(tls.RequireAndVerifyClientCert)
```

All settings support runtime updates, just like certificate rotation.

For detailed configuration and examples, see [Mutual TLS](../networking/mutual-tls.md).

For complete certificate manager methods and usage, refer to the `gen.CertManager` interface documentation in the code.

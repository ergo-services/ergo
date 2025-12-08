---
description: Mutual TLS authentication between nodes
---

# Mutual TLS

Standard TLS provides server authentication - the client verifies the server's certificate. Mutual TLS (mTLS) adds client authentication - both sides present and verify certificates. Only clients with certificates signed by a trusted CA can connect.

## Configuration

```go
func startSecureNode(name string) (gen.Node, error) {
    // Load node certificate (signed by cluster CA)
    cert, err := tls.LoadX509KeyPair(
        fmt.Sprintf("%s.pem", name),
        fmt.Sprintf("%s-key.pem", name),
    )
    if err != nil {
        return nil, err
    }

    // Load cluster CA
    caCert, err := os.ReadFile("cluster-ca.pem")
    if err != nil {
        return nil, err
    }
    caPool := x509.NewCertPool()
    caPool.AppendCertsFromPEM(caCert)

    certManager := gen.CreateCertAuthManager(cert)
    certManager.SetClientCAs(caPool)                          // verify incoming
    certManager.SetClientAuth(tls.RequireAndVerifyClientCert) // require client cert
    certManager.SetRootCAs(caPool)                            // verify outgoing

    return ergo.StartNode(gen.Atom(name), gen.NodeOptions{
        CertManager: certManager,
    })
}
```

`NodeOptions.CertManager` is used for:
- Default acceptor (created automatically on port 15000)
- All outgoing connections

To override per-acceptor, use `AcceptorOptions.CertManager`.

## CertAuthManager

`gen.CertAuthManager` extends `CertManager` with CA pool and authentication settings:

```go
type CertAuthManager interface {
    CertManager

    // server-side
    SetClientCAs(pool *x509.CertPool)
    SetClientAuth(auth tls.ClientAuthType)

    // client-side
    SetRootCAs(pool *x509.CertPool)
    SetServerName(name string) // for SNI
}
```

**Server-side settings:**

| Setting | Purpose |
|---------|---------|
| `ClientCAs` | CA pool to verify client certificates |
| `ClientAuth` | How strictly to enforce client certificates |

**ClientAuth values:**

| Value | Behavior |
|-------|----------|
| `tls.NoClientCert` | Don't request client certificate (default) |
| `tls.RequestClientCert` | Request but don't require |
| `tls.RequireAnyClientCert` | Require certificate, don't verify against CA |
| `tls.VerifyClientCertIfGiven` | Verify against CA if provided |
| `tls.RequireAndVerifyClientCert` | Require and verify against CA |

**Client-side settings:**

| Setting | Purpose |
|---------|---------|
| `RootCAs` | CA pool to verify server certificates |
| `ServerName` | Server name for SNI (if different from host) |

## Runtime Certificate Rotation

Certificates can be rotated without restart:

```go
newCert, _ := tls.LoadX509KeyPair("new.pem", "new-key.pem")
certManager.Update(newCert)
```

New connections use the updated certificate. Existing connections keep their original certificate.

CA pools and `ClientAuth` are fixed at startup. Restart the node to change these settings.

To use different certificates for specific destinations, see [Static Routes](static-routes.md).

## Troubleshooting

**Connection rejected with certificate error**

Verify the client certificate is signed by a CA in the server's `ClientCAs` pool. Check certificate expiration dates.

**Server certificate verification failed**

The server's certificate must be signed by a CA in the client's `RootCAs` pool. For development, disable verification with `NetworkOptions.InsecureSkipVerify: true`.

**SNI mismatch**

Set `ServerName` on the client's `CertAuthManager` if the certificate's Common Name doesn't match the connection address.

**Certificate rotation not taking effect**

Updates apply to new connections only. Close existing connections to force reconnection with new certificate.

**CA pool changes not taking effect**

CA pools are fixed at startup. Restart the node to apply changes.

# CertManager

Ergo Framework introduces `gen.CertManager` to simplify TLS certificate management. It enables live certificate updates without restarting the application and can be used across all network components like node acceptors, web, or TCP servers.&#x20;

Use the function `gen.CreateCertManager(cert tls.Certificate)` to create a certificate manager. The `gen.CertManager` interface provides the following methods:

* **Update**: Update the TLS certificate on the fly. All components using this `CertManager` will automatically receive the updated certificate.
* **GetCertificateFunc**: Returns a closure to retrieve the TLS certificate.
* **GetCertificate**: Returns the current TLS certificate.

Additionally, you can use the `lib.GenerateSelfSignedCert` function to generate a self-signed certificate.

```go
import (
    ...
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
    "ergo.services/ergo/lib"
    ...
)

var (
    Version = gen.Version{
        Name:    "DemoService",
	Release: "1.0",
    }
)

func main() {
    var options gen.NodeOptions
    ...
    cert, err := lib.GenerateSelfSignedCert(Version.String())
    if err != nil {
        panic(err)
    }
    options.CertManager = gen.CreateCertManager(cert)
    ...
    node, err := ergo.StartNode("node@localhost", options)
    if err != nil {
	panic(err)
    }

    node.Wait()
} 
```


# Third-Party Notices

The `mongo` module depends on the official MongoDB Go Driver module `go.mongodb.org/mongo-driver/v2` at the locked version `v2.8.2`.

| Project | Version | License | Upstream notices |
| --- | --- | --- | --- |
| MongoDB Go Driver | `v2.8.2` | [Apache License 2.0](https://github.com/mongodb/mongo-go-driver/blob/v2.8.2/LICENSE) | [THIRD-PARTY-NOTICES](https://github.com/mongodb/mongo-go-driver/blob/v2.8.2/THIRD-PARTY-NOTICES) |

The driver source states `Copyright (C) MongoDB, Inc. 2017-present`. The pinned upstream notice retains additional terms for incorporated code and dependencies, including AWS V4 signing code, `gopkg.in/mgo.v2/bson`, Go project JSON, CSV, arithmetic code, `golang.org/x/exp/rand`, compression packages, XDG authentication packages, and other named Go dependencies. The exact locked upstream notice file is the authoritative complete list; this summary is not a substitute for it.

The MongoDB Go Driver is independent from Weave and the Weave MongoDB Adapter. Its project and product names are used only to identify a dependency and compatibility boundary and do not imply sponsorship or endorsement.

Repository-owned Adapter code is covered by the repository's Apache License 2.0. The upstream driver license and notices remain applicable to the driver code; this file does not replace or modify them.

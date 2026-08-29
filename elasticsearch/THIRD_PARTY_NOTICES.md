# Third-Party Notices

The `elasticsearch` module depends on the official Elastic Go client and its
transport at the locked versions below.

| Project | Version | License | Source |
| --- | --- | --- | --- |
| go-elasticsearch | `v9.5.1` | Apache License 2.0 | [github.com/elastic/go-elasticsearch](https://github.com/elastic/go-elasticsearch/tree/v9.5.1) |
| elastic-transport-go | `v8.9.0` | Apache License 2.0 | [github.com/elastic/elastic-transport-go](https://github.com/elastic/elastic-transport-go/tree/v8.9.0) |

Elastic and these projects are independent from Weave and the Weave
Elasticsearch Adapter. Their names identify dependencies and do not imply
sponsorship or endorsement.

Repository-owned Adapter code is covered by the repository's Apache License
2.0. The exact upstream licenses remain applicable to the dependency code. The
attribution text shipped in the locked go-elasticsearch `NOTICE` is:

```text
Elasticsearch Go Client
Copyright 2021 Elasticsearch B.V.
```

The attribution text shipped in the locked elastic-transport-go `NOTICE` is:

```text
Elastic Transport Go
Copyright 2014-2021 Elasticsearch BV

This product includes software developed by The Apache Software
Foundation (http://www.apache.org/).
```

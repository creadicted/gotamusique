# Vendored Swagger UI

`swagger-ui-bundle.js`, `swagger-ui.css`, `LICENSE` and `NOTICE` are copied unchanged from
the npm package **swagger-ui-dist 5.34.1** (Apache-2.0, SmartBear Software).

    npm pack swagger-ui-dist@5.34.1
    integrity sha512-VSr8fXfI93obYaQicLJbLUKGRxDojj/SfRJDDLCioGTUBj4BK/Nj4eE9jCeCAnJxVLi7iS+08PbiZLyrQKBkJQ==

`index.html` and `init.js` are ours. They load the bundle from `/api/docs/assets/` and point
it at `/api/v1/openapi.yaml`.

To update: `npm pack swagger-ui-dist@<version>`, extract the tarball, copy the four files over
these, and change the version and integrity above. `TestDocs_VendoredFilesAreUnmodified` pins the
sha256 of the bundle and the stylesheet so an edit cannot slip in unnoticed; put the new hashes
(`sha256sum`) in that test. The bundle contains one `new Function(...)`
(webpack's global-object lookup, wrapped in a `try` with a fallback), so the page's CSP does not
need `unsafe-eval`; check that still holds after an update.

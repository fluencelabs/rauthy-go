# Changelog

## 0.1.0 (2026-11-15)


### Features

* **client:** publish the Rauthy API client as package rauthy ([4f8da35](https://github.com/fluencelabs/rauthy-go/commit/4f8da351b075122ab88184988c25e3dee0d14307))


### Bug Fixes

* **auth-providers:** report an ambiguous name as ErrAmbiguousName, not a 404 ([4968531](https://github.com/fluencelabs/rauthy-go/commit/496853189c886dd6a3cc66e652e3b87002350ad4))
* **client:** harden redirects, retries and path handling ([8042997](https://github.com/fluencelabs/rauthy-go/commit/8042997ca29d41274dc99eaec4b4e852dd3abca5))
* **client:** harden redirects, retries and path handling ([0463f1b](https://github.com/fluencelabs/rauthy-go/commit/0463f1bf62b51db99d3884a5c397c0aa4f57ef1d))
* **client:** say where a refused redirect pointed ([ef8b9dd](https://github.com/fluencelabs/rauthy-go/commit/ef8b9dd2c4b34e24899c98558aa05c14b9b228c6))
* **clients:** model every field of the client full-replace body ([21e0ac3](https://github.com/fluencelabs/rauthy-go/commit/21e0ac305b6af7283978eb83aa85f6a9ae497fec))
* **clients:** model every field of the client full-replace body ([93d3300](https://github.com/fluencelabs/rauthy-go/commit/93d3300ef184cb6ee3fc34ec080b4d6044b5eabb))
* **retry:** do not retry an attempt cut off by the http.Client timeout ([31d59ca](https://github.com/fluencelabs/rauthy-go/commit/31d59ca3b2fb7eaf4201e8b748853910ed898398))
* **scopes:** carry claims_at_root through a read-modify-write ([2a6d78d](https://github.com/fluencelabs/rauthy-go/commit/2a6d78d11f42be58cf0ffe73c0a1a1d801ab8395))
* **scripts:** leave the spec untouched when openapi-refresh fails ([97cffb8](https://github.com/fluencelabs/rauthy-go/commit/97cffb8ef170e82270f4da258786a719caa14ae2))

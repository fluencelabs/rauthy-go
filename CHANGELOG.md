# Changelog

## [0.2.0](https://github.com/fluencelabs/rauthy-go/compare/v0.1.0...v0.2.0) (2026-11-16)


### Features

* **client:** report the Rauthy server version ([2ed61cd](https://github.com/fluencelabs/rauthy-go/commit/2ed61cd57d6d38b3703e319f0d12d10350cb1780))
* **client:** report the Rauthy server version ([1f0a66c](https://github.com/fluencelabs/rauthy-go/commit/1f0a66c9b7594a42e4702d6075b1e46a30729e36))
* model the remaining role, group and user fields ([e473a42](https://github.com/fluencelabs/rauthy-go/commit/e473a420d00e8b9b6aeb59c3d2ddc21ed5e78994))
* **roles,groups,users:** expose meta, read-only user fields and UpdateRequest helpers ([ed05a14](https://github.com/fluencelabs/rauthy-go/commit/ed05a149141dcc996b3c4f69c308c89d4e579ddc))


### Bug Fixes

* align client value constraints with Rauthy ([56a2bdd](https://github.com/fluencelabs/rauthy-go/commit/56a2bdd17f1ce949459d7b2fc6fda9fe4ccdeae9))
* align client value constraints with Rauthy ([15b0b21](https://github.com/fluencelabs/rauthy-go/commit/15b0b215ee547fc86093d69f8e1cd13b77e09c7e))
* keep rejected values out of APIError ([e7327ee](https://github.com/fluencelabs/rauthy-go/commit/e7327eed15c5b27be99159c8ea1357b9ff49ded0))
* keep rejected values out of APIError ([7fed14d](https://github.com/fluencelabs/rauthy-go/commit/7fed14de5778fe3111d0e5f94bbf4546d331f94b))
* model the remaining full-replace request fields ([74b8bb0](https://github.com/fluencelabs/rauthy-go/commit/74b8bb0c7b103aaf67960872dee943fa5dc6aa21))
* **user-attributes:** never send typ ([019f74d](https://github.com/fluencelabs/rauthy-go/commit/019f74d3febb3301fc8d6795645235128862c598))

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

# Changelog

## [0.3.1](https://github.com/fluencelabs/rauthy-go/compare/v0.3.0...v0.3.1) (2026-11-17)


### Bug Fixes

* **retry:** do not replay a PUT after a 500 ([7f2aafa](https://github.com/fluencelabs/rauthy-go/commit/7f2aafa68a769ade8a4d36f58705e9080a634eee))
* **retry:** do not replay a PUT whose SCIM sync failed ([054b838](https://github.com/fluencelabs/rauthy-go/commit/054b838e38dfaa447a9633d8d7b62f2dd825329e))
* **retry:** replay a PUT after any 500 except a failed SCIM sync ([85758d3](https://github.com/fluencelabs/rauthy-go/commit/85758d3af8e8b352d4c0f1e312cca065598e75cf))
* serialise client updates with scope calls ([5c4c3c6](https://github.com/fluencelabs/rauthy-go/commit/5c4c3c64cabc6329ce62afa7591be97c316efe20))
* serialise scope calls ([e07d7a7](https://github.com/fluencelabs/rauthy-go/commit/e07d7a72ad5474d9325b1b2c84c9afc2b14c49e1))
* serialise scope reads and attribute writes with scope writes ([4f7ac38](https://github.com/fluencelabs/rauthy-go/commit/4f7ac38cc2168cfdce38307e7ab85a2a610acb95))
* serialise scope writes ([87bf712](https://github.com/fluencelabs/rauthy-go/commit/87bf712b15128a611a767aee0fa3314cedb5b418))
* serialise user attribute reads with the writes ([c3481f3](https://github.com/fluencelabs/rauthy-go/commit/c3481f3e395e82718e605fdc4c82b4fa54e0019e))

## [0.3.0](https://github.com/fluencelabs/rauthy-go/compare/v0.2.0...v0.3.0) (2026-11-17)


### Bug Fixes

* **contract:** decode linked users on 406 and canonicalize IP deletion ([7bb32d7](https://github.com/fluencelabs/rauthy-go/commit/7bb32d7ac9ce09b49b14ea4f1122c0ba31f69528))
* keep the base URL out of transport errors ([13addde](https://github.com/fluencelabs/rauthy-go/commit/13addde044cf3adad0974d9bd5b0e4ce01710965))
* name only the scheme and host of a refused redirect ([bfacbff](https://github.com/fluencelabs/rauthy-go/commit/bfacbff0a94597c19be68696733e8b447ccc5b56))
* redact credentials in formatting, stop replaying password changes, decode 406 linked users ([f3db97b](https://github.com/fluencelabs/rauthy-go/commit/f3db97b939424e44b0dad3c0942141109859ec37))
* **retry:** avoid replaying password changes and honor queued deadlines ([e2df34e](https://github.com/fluencelabs/rauthy-go/commit/e2df34e1f28268388b967f10041392230596b1d1))
* return the base URL as a Secret ([ec93f66](https://github.com/fluencelabs/rauthy-go/commit/ec93f662aeea2f3f1b269799052edc2301609f69))
* **security:** redact credential formatting and URL configuration errors ([abf3b7d](https://github.com/fluencelabs/rauthy-go/commit/abf3b7dad5bcbfea81babecc07b3f6fbfc15eef6))
* **security:** store credentials as indirect redacted secrets ([12543bc](https://github.com/fluencelabs/rauthy-go/commit/12543bc9e2326fc70d1ede9c7626ba5832a2d4c4))


### Miscellaneous Chores

* release 0.3.0 ([575371d](https://github.com/fluencelabs/rauthy-go/commit/575371d4087ea4bee8cfd2137c7eae03ac13042c))

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

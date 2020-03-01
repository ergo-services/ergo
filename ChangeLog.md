# Changelog
All notable changes to this project will be documented in this file.

This format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

#### [1.0.0](https://github.com/halturin/ergo/releases/tag/1.0.0) - 2019-11-30 ####
## There is a bunch of changes we deliver with this release
- We have changed the name - Ergo (or Ergo Framework). GitHub's repo has been 
renamed as well. We also created cloned repo `ergonode` to support users of 
the old version of this project. So, its still available at 
https://github.com/halturin/ergonode. But it's strongly recommend to use 
the new one.
- Completely reworked (almost from scratch) architecture whole project
- Implemented linking process feature (in order to support Application/Supervisor behaviors)
- Reworked Monitor-feature. Now it has full-featured support with remote process/nodes
- Added multinode support
- Added experimental observer support
- Fixed incorrect ETF string encoding
- Improved ETF TermIntoStruct decoder
- Improved code structure and readability

#### [0.2.0](https://github.com/halturin/ergonode/releases/tag/0.2.0) - 2019-02-23 ####
- Now we make versioning releases
- Improve node creation. Now you can specify the listening port range. See 'Usage' for details
- Add embedded EPMD. Trying to start internal epmd service on starting ergonode.

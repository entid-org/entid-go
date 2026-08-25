# Copyright The EntID Authors.
# SPDX-License-Identifier: Apache-2.0

.PHONY: verify

# verify is the only entry point. Section 12.6 of engine.md asks for one, and
# for a reason worth repeating here: a resync round is about thirty commands,
# twenty-nine of whose outputs say nothing but "this passes". Run this instead,
# and read the failing step when there is one.
verify:
	@./scripts/verify.sh

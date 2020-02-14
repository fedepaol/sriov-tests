.PHONY: e2e operator conformance deps-update

operator:
	./scripts/run-test.sh operator

e2e:
	./scripts/run-test.sh e2e

deps-update:
	go mod tidy && \
	go mod vendor

conformance:
	./scripts/run-conformance.sh


GEN=go generate ./...
CHECK=git diff --quiet -- . ':(exclude)**/*.sum' || echo "generated code not up-to-date"

.PHONY: gen check
gen:
	$(GEN)

check:
	$(GEN)
	$(CHECK)

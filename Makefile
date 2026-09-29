# Builds each Lambda as an arm64 `bootstrap` binary for the provided.al2023 runtime, zipped into dist/.
FUNCTIONS := web worker

.PHONY: dist clean
dist: $(FUNCTIONS:%=dist/%.zip)

dist/%.zip: $(shell find cmd internal -name '*.go') go.mod go.sum
	@mkdir -p dist/$*
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -ldflags='-s -w' -o dist/$*/bootstrap ./cmd/$*
	cd dist/$* && rm -f ../$*.zip && zip -q -X ../$*.zip bootstrap

clean:
	rm -rf dist

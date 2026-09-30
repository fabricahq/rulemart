# Local build of the release assets CI publishes: one ZIP per function under cmd/, plus SHA256SUMS and manifest.json.
.PHONY: dist check clean
dist:
	python3 scripts/package-release.py --commit "$$(git rev-parse HEAD)" --output dist

check:
	go vet ./...
	go test ./...

clean:
	rm -rf dist

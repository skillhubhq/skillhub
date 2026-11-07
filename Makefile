.PHONY: web proto-deps proto proto-check test check build desktop sdk-smoke quality

proto-deps:
	npm --prefix sdk/typescript ci --ignore-scripts

proto: proto-deps
	buf format -w
	buf lint
	buf generate

proto-check: proto
	git diff --exit-code -- gen sdk/typescript/gen
	test -z "$$(git ls-files --others --exclude-standard gen sdk/typescript/gen)"

test:
	go test -race ./...

check: proto-deps
	buf format --diff --exit-code
	buf lint
	buf build
	go vet ./...
	go test -race ./...
	npm --prefix sdk/typescript test

web:
	npm --prefix desktop/frontend ci --ignore-scripts
	npm --prefix sdk/typescript ci --ignore-scripts
	npm --prefix desktop/frontend run build:web

build: web
	go build ./...

desktop: proto-deps
	cd desktop/frontend && npm ci --ignore-scripts
	cd desktop && node build.mjs

sdk-smoke:
	go build -o bin/skillhub-server ./cmd/skillhub-server
	npm --prefix sdk/typescript run smoke -- ../../bin/skillhub-server

quality:
	python3 scripts/check_quality.py
	python3 -m unittest discover -s scripts -p "test_*.py"

.PHONY: build serve test lint tf-fmt tf-validate check verify new clean

build:            ## Render the site into ./public
	go run ./cmd/garage build

serve:            ## Build with drafts and serve on http://127.0.0.1:8080
	go run ./cmd/garage serve

new:              ## make new TITLE="My post"
	go run ./cmd/garage new "$(TITLE)"

test:
	go test -race -cover ./...

lint:
	golangci-lint run

tf-fmt:
	terraform -chdir=infra/terraform fmt -recursive

tf-validate:
	terraform -chdir=infra/terraform init -backend=false -input=false >/dev/null
	terraform -chdir=infra/terraform validate

check: test lint tf-validate   ## What CI runs

verify:           ## Read-only check of GitHub env + AWS + live site wiring
	scripts/verify-deploy-setup.sh

clean:
	rm -rf public

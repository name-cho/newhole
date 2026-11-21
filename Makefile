.PHONY: build-frontend build-cli build-desktop build-armv5 test fmt vet clean

build-frontend:
	$(MAKE) -C ui/desktop install build

build-cli:
	go build -tags cli -o newhole .

build-desktop: build-frontend
	go build -tags desktop -o newhole-desktop .

build-armv5:
	GOOS=linux GOARCH=arm GOARM=5 go build -tags cli -o newhole-linux-armv5 .

test:
	go test -tags cli ./...

fmt:
	gofmt -l -w .

vet:
	go vet -tags cli ./...

clean:
	rm -f newhole newhole-desktop
	rm -rf dist

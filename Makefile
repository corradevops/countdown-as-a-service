BINARY_NAME=caas-server

build:
	GOARCH=amd64 GOOS=darwin go build -o ${BINARY_NAME}-darwin-amd64 main.go
	GOARCH=arm64 GOOS=darwin go build -o ${BINARY_NAME}-darwin-arm64 main.go
	GOARCH=amd64 GOOS=linux go build -o ${BINARY_NAME}-linux-amd64 main.go
	GOARCH=arm64 GOOS=linux go build -o ${BINARY_NAME}-linux-arm64 main.go
	GOARCH=amd64 GOOS=windows go build -o ${BINARY_NAME}-windows.exe main.go

run: build
	./${BINARY_NAME}-linux

clean:
	go clean
	@if [ -f "${BINARY_NAME}-darwin-amd64" ]; then \
		rm ${BINARY_NAME}-darwin-amd64; \
	fi
	@if [ -f "${BINARY_NAME}-darwin-arm64" ]; then \
		rm ${BINARY_NAME}-darwin-arm64; \
	fi
	@if [ -f "${BINARY_NAME}-linux-amd64" ]; then \
		rm ${BINARY_NAME}-linux-amd64; \
	fi
	@if [ -f "${BINARY_NAME}-linux-arm64" ]; then \
		rm ${BINARY_NAME}-linux-arm64; \
	fi
	@if [ -f "${BINARY_NAME}-windows.exe" ]; then \
		rm "${BINARY_NAME}-windows.exe"; \
	fi

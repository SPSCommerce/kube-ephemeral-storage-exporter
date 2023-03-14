.PHONY:
	local
	run
# The version that will be used in docker tags
VERSION ?= $(shell git rev-parse --short HEAD)

ENTRY_POINT=./cmd

image: build_ci # Build executable and docker image
	docker build -t kube-hpa-scale-to-zero:$(VERSION) .

local: # Run go application locally
	go run ${ENTRY_POINT}

run: image # Run docker container in foreground
	docker run -p 8080:8080 kube-hpa-scale-to-zero:$(VERSION)
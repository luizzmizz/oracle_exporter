BINARY := /tmp/oracle_exporter_oel9
IMAGE  := oracle-exporter-build

.PHONY: build clean

build:
	docker build -f Dockerfile.build -t $(IMAGE) .
	docker run --rm $(IMAGE) cat /build/oracle_exporter > $(BINARY)
	chmod +x $(BINARY)
	@echo "Built: $(BINARY)"

clean:
	docker rmi -f $(IMAGE) 2>/dev/null || true

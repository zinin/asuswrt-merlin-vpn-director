.PHONY: web web-embed build-webui build-webui-arm64 build-webui-arm build-bot build-bot-arm64 build-bot-arm build-all clean

# Build Vue SPA
web:
	cd web && npm ci && npm run build

# Copy dist into Go embed location
web-embed: web
	rm -rf server/cmd/webui/web/dist
	mkdir -p server/cmd/webui/web
	cp -r web/dist server/cmd/webui/web/dist

# Build webui binary (requires web-embed first)
build-webui: web-embed
	make -C server build-webui

build-webui-arm64: web-embed
	make -C server build-webui-arm64

build-webui-arm: web-embed
	make -C server build-webui-arm

# Build bot (unchanged)
build-bot:
	make -C server build

build-bot-arm64:
	make -C server build-arm64

build-bot-arm:
	make -C server build-arm

# Build all
build-all: build-webui-arm64 build-webui-arm build-bot-arm64 build-bot-arm

clean:
	rm -rf web/dist server/cmd/webui/web/dist
	make -C server clean

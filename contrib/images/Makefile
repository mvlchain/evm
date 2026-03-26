all: tadad-env

tadad-env: tadad-rmi
	docker build --tag cosmos/tadad -f evmd-env/Dockerfile \
		$(shell git rev-parse --show-toplevel)

tadad-rmi:
	docker rmi cosmos/tadad 2>/dev/null; true

.PHONY: all tadad-env tadad-rmi

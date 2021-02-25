export COMPOSE_FILE = docker/dev.yml

dev:
	@touch creds.yml
	@docker build -t strife-app --target bin .
	@docker run --rm -it strife-app
live:
	@touch creds.yml
	@docker build -t strife-app --target bin /strife
	@docker run strife-app

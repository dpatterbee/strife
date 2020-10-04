export COMPOSE_FILE = docker/dev.yml

dev:
	@touch creds.yml
	@docker-compose rm -f 
	@docker-compose up --build
live:
	@touch creds.yml
	@docker build -t strife-app --target bin .
	@docker run strife-app

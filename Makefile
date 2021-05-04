export COMPOSE_FILE = docker/dev.yml

build:
	@DOCKER_BUILDKIT=1 docker build -t strife .

run:
	@docker run -e "LITESTREAM_ACCESS_KEY_ID=${LITESTREAM_ACCESS_KEY_ID}" \
  		-e "LITESTREAM_SECRET_ACCESS_KEY=${LITESTREAM_SECRET_ACCESS_KEY}" \
  		-e "DB_REPLICA_URL=${DB_REPLICA_URL}" \
  		-e "DB_PATH"=/data/store.db \
  		-e "TOKEN"=${TOKEN} \
		-dit strife

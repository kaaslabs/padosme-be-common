# Host Installation Guide — PostgreSQL, Redis, RabbitMQ, MongoDB

Install these on the host (not in containers) for staging and production.

---

## Architecture

```
Host Machine
├── PostgreSQL 16+     (port 5432)  — all service databases
├── Redis 7+           (port 6379)  — caching, geo, rate limiting
├── RabbitMQ 3.13+     (port 5672)  — event bus
├── MongoDB 7+         (port 27017) — catalogue-service only
│
├── Docker containers:
│   ├── OTEL Collector  (port 4317)
│   ├── Jaeger          (port 16686)
│   ├── Prometheus      (port 9090)
│   ├── Grafana         (port 3000)
│   └── 20 business services (ports 8070-8097)
```

---

## 1. PostgreSQL 16

### Ubuntu/Debian
```bash
# Install
sudo apt-get install -y postgresql-16 postgresql-client-16

# Start and enable
sudo systemctl enable postgresql
sudo systemctl start postgresql

# Create deploy user
sudo -u postgres psql -c "CREATE USER deploy WITH PASSWORD 'YOUR_STRONG_PASSWORD' CREATEDB;"

# Enable SSL
sudo -u postgres psql -c "ALTER SYSTEM SET ssl = on;"

# Generate self-signed cert (replace with proper cert for production)
sudo openssl req -new -x509 -days 365 -nodes \
  -out /etc/postgresql/16/main/server.crt \
  -keyout /etc/postgresql/16/main/server.key \
  -subj "/CN=padosme-db"
sudo chown postgres:postgres /etc/postgresql/16/main/server.{crt,key}
sudo chmod 600 /etc/postgresql/16/main/server.key

# Restart to apply SSL
sudo systemctl restart postgresql
```

### postgresql.conf tuning
```ini
# Connection
listen_addresses = 'localhost'       # or '0.0.0.0' if services run on other machines
max_connections = 200                # 20 services × ~10 connections each
port = 5432

# SSL
ssl = on
ssl_cert_file = '/etc/postgresql/16/main/server.crt'
ssl_key_file = '/etc/postgresql/16/main/server.key'

# Memory (adjust for your server RAM — example for 8GB)
shared_buffers = 2GB                 # 25% of RAM
effective_cache_size = 6GB           # 75% of RAM
work_mem = 16MB                      # per-sort/hash operation
maintenance_work_mem = 512MB         # for VACUUM, CREATE INDEX

# WAL
wal_buffers = 64MB
max_wal_size = 2GB
min_wal_size = 512MB
checkpoint_completion_target = 0.9

# Planner
random_page_cost = 1.1               # SSD storage
effective_io_concurrency = 200       # SSD storage
default_statistics_target = 100

# Logging
logging_collector = on
log_directory = 'log'
log_min_duration_statement = 500     # log queries > 500ms
log_checkpoints = on
log_connections = on
log_disconnections = on
log_lock_waits = on

# Autovacuum
autovacuum_max_workers = 4
autovacuum_naptime = 30s
```

### pg_hba.conf (authentication)
```
# TYPE  DATABASE   USER    ADDRESS        METHOD
local   all        all                    peer
host    all        deploy  127.0.0.1/32   scram-sha-256
host    all        deploy  ::1/128        scram-sha-256
hostssl all        deploy  0.0.0.0/0      scram-sha-256    # remote with SSL only
```

### Create databases
```bash
# Use the setup script
PGHOST=localhost PGPORT=5432 PGUSER=deploy PGPASSWORD='YOUR_PASSWORD' \
  ./scripts/setup-databases.sh
```

---

## 2. Redis 7

### Ubuntu/Debian
```bash
# Install
sudo apt-get install -y redis-server

# Enable and start
sudo systemctl enable redis-server
sudo systemctl start redis-server
```

### /etc/redis/redis.conf tuning
```ini
# Network
bind 127.0.0.1 ::1
port 6379
protected-mode yes

# Authentication
requirepass YOUR_REDIS_PASSWORD

# Memory
maxmemory 2gb
maxmemory-policy allkeys-lru

# Persistence (AOF for durability)
appendonly yes
appendfsync everysec
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb

# Also keep RDB snapshots (belt + suspenders)
save 900 1
save 300 10
save 60 10000

# Performance
tcp-keepalive 300
timeout 0
hz 10

# Limits
maxclients 1000

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log
```

### Verify
```bash
redis-cli -a YOUR_REDIS_PASSWORD ping
# → PONG
```

---

## 3. RabbitMQ 3.13

### Ubuntu/Debian
```bash
# Add Erlang + RabbitMQ repos (official)
# See: https://www.rabbitmq.com/docs/install-debian

# Install
sudo apt-get install -y rabbitmq-server

# Enable and start
sudo systemctl enable rabbitmq-server
sudo systemctl start rabbitmq-server

# Enable plugins
sudo rabbitmq-plugins enable rabbitmq_management
sudo rabbitmq-plugins enable rabbitmq_prometheus
sudo rabbitmq-plugins enable rabbitmq_consistent_hash_exchange

# Delete default guest user
sudo rabbitmqctl delete_user guest

# Create admin user
sudo rabbitmqctl add_user rmq_admin 'YOUR_ADMIN_PASSWORD'
sudo rabbitmqctl set_user_tags rmq_admin administrator
sudo rabbitmqctl set_permissions -p / rmq_admin ".*" ".*" ".*"

# Create application user
sudo rabbitmqctl add_user padosme_app 'YOUR_APP_PASSWORD'
sudo rabbitmqctl set_user_tags padosme_app ''
sudo rabbitmqctl set_permissions -p / padosme_app ".*" ".*" ".*"

# Set file descriptor limit
sudo mkdir -p /etc/systemd/system/rabbitmq-server.service.d
echo -e "[Service]\nLimitNOFILE=65536" | sudo tee /etc/systemd/system/rabbitmq-server.service.d/limits.conf
sudo systemctl daemon-reload
sudo systemctl restart rabbitmq-server
```

### /etc/rabbitmq/rabbitmq.conf
Copy the `rabbitmq.conf` file from this directory.

### /etc/rabbitmq/advanced.config
Copy the `advanced.config` file from this directory.

### Verify
```bash
# Management UI
curl -u rmq_admin:YOUR_ADMIN_PASSWORD http://localhost:15672/api/overview | jq .
```

---

## 4. MongoDB 7 (catalogue-service only)

### Ubuntu/Debian
```bash
# Add MongoDB repo
# See: https://www.mongodb.com/docs/manual/tutorial/install-mongodb-on-ubuntu/

# Install
sudo apt-get install -y mongodb-org

# Enable and start
sudo systemctl enable mongod
sudo systemctl start mongod

# Create admin user
mongosh --eval '
  db = db.getSiblingDB("admin");
  db.createUser({
    user: "deploy",
    pwd: "YOUR_MONGO_PASSWORD",
    roles: [{ role: "readWriteAnyDatabase", db: "admin" }]
  });
'

# Create catalogue database
mongosh -u deploy -p YOUR_MONGO_PASSWORD --authenticationDatabase admin --eval '
  db = db.getSiblingDB("catalog_db");
  db.createCollection("items");
'
```

### /etc/mongod.conf
```yaml
storage:
  dbPath: /var/lib/mongodb
  journal:
    enabled: true

net:
  port: 27017
  bindIp: 127.0.0.1

security:
  authorization: enabled
```

---

## 5. OS-Level Tuning

```bash
# /etc/sysctl.conf — add these
cat >> /etc/sysctl.conf << 'EOF'
# Network
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 4096
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_tw_reuse = 1
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216

# File descriptors
fs.file-max = 262144

# Virtual memory
vm.overcommit_memory = 1
vm.swappiness = 10
EOF

sudo sysctl -p

# File descriptor limits
cat >> /etc/security/limits.conf << 'EOF'
deploy    soft    nofile    65536
deploy    hard    nofile    65536
postgres  soft    nofile    65536
postgres  hard    nofile    65536
rabbitmq  soft    nofile    65536
rabbitmq  hard    nofile    65536
redis     soft    nofile    65536
redis     hard    nofile    65536
EOF
```

---

## 6. Start Everything

```bash
# 1. Verify host services are running
sudo systemctl status postgresql redis-server rabbitmq-server mongod

# 2. Create Docker network
docker network create padosme-network

# 3. Run databases setup (creates all 17 databases + migrations)
PGHOST=localhost PGPORT=5432 PGUSER=deploy PGPASSWORD='YOUR_PASSWORD' \
  ./scripts/setup-databases.sh

# 4. Start observability stack
docker compose -f docker-compose.prod.yml up -d

# 5. (Optional) Start admin tools
docker compose -f docker-compose.admin-tools.yml up -d

# 6. Deploy business services (each service's own docker-compose.prod.yml)
# This is handled by the CI/CD pipeline per service
```

---

## Compose Files Summary

| File | What | When to Use |
|------|------|-------------|
| `docker-compose.yml` | Everything in containers | Local dev only |
| `docker-compose.prod.yml` | Observability only (OTEL, Jaeger, Prometheus, Grafana) | Staging + Production |
| `docker-compose.observability.yml` | Same as prod (standalone) | If you want observability without prod overrides |
| `docker-compose.admin-tools.yml` | pgAdmin + RedisInsight | On-demand debugging |

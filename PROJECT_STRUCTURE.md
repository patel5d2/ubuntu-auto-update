# Ubuntu Auto-Update Project Structure

## 🗂️ **Core Components**

```
ubuntu-auto-update/
├── 🦀 agent/                 # Rust system agent
│   ├── src/                  # Rust source code
│   ├── docker/               # Container configuration
│   ├── systemd/              # Service definitions
│   └── Cargo.toml            # Rust project config
│
├── 🐹 backend/               # Go API server
│   ├── cmd/api/              # Main application
│   ├── pkg/                  # Shared packages (models, db, etc.)
│   ├── db/migrations/        # Database schema migrations
│   └── go.mod                # Go module definition
│
├── ⚛️  web/                   # React dashboard
│   ├── src/                  # React TypeScript source
│   ├── package.json          # Node.js dependencies
│   └── vite.config.ts        # Build configuration
│
├── 🐳 docker-compose.dev.yml # Development environment
├── 📜 scripts/               # Build and test automation
└── 📚 Documentation files
```

## 🚀 **Quick Start**

```bash
# 1. Start infrastructure
docker-compose -f docker-compose.dev.yml up -d postgres redis

# 2. Start backend
cd backend
export DATABASE_URL="postgres://user:password@localhost:5432/uau_db?sslmode=disable"
export ADMIN_USERNAME="admin"
export ADMIN_PASSWORD="password"
go run cmd/api/main.go

# 3. Start frontend
cd ../web
npm install
npm run dev

# 4. Access dashboard at http://localhost:3000
```

## 🔧 **Build Commands**

```bash
./scripts/build.sh --all        # Build all components
./scripts/test.sh --unit        # Run tests
```

## 🌟 **Key Features**

- ✅ **Enterprise Dashboard**: React + TypeScript web interface
- ✅ **Secure API**: Go backend with PostgreSQL
- ✅ **Memory-Safe Agent**: Rust system agent
- ✅ **Real-time Updates**: WebSocket communication
- ✅ **SSH Management**: Passwordless remote execution
- ✅ **Container Support**: Docker development environment

## 🎯 **Current Status**

**Working Components:**
- Dashboard login and authentication ✅
- Host management interface ✅
- Remote SSH command execution ✅
- Database persistence ✅
- WebSocket real-time updates ✅

**Demonstration Ready:**
- Login to dashboard with admin/password
- View registered Ubuntu hosts
- Execute remote update commands
- Monitor live command output
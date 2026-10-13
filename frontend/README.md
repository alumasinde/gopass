# GoPass Platform Admin UI

This is the Platform Admin interface for managing tenant organizations in the GoPass system.

## Features

- **Authentication**: Secure login for platform administrators using JWT tokens
- **Dashboard**: Overview of all tenant organizations
- **Tenant Management**:
  - Create new tenants with admin accounts
  - View tenant details (lifecycle status, provisioning status, database mode)
  - Suspend/Resume active tenants
  - Deprovision tenants
  - Retry failed provisioning

## Getting Started

### Prerequisites

- Backend API running on `http://localhost:8080` (or configure via `VITE_API_BASE_URL`)
- A platform admin account created in the database

### Installation

1. Install dependencies:
```bash
npm install
```

2. Configure environment:
```bash
cp .env.example .env
# Edit .env to set your API base URL if different from localhost:8080
```

3. Start the development server:
```bash
npm run dev
```

4. Open your browser to `http://localhost:5173`

### Backend Integration

The frontend integrates with the following backend endpoints:

**Authentication:**
- `POST /platform/auth/login` - Platform admin login
- `POST /platform/auth/logout` - Platform admin logout

**Tenant Management:**
- `GET /platform/organizations` - List all organizations
- `GET /platform/organizations/{id}` - Get organization details
- `POST /platform/organizations` - Create new organization
- `PATCH /platform/organizations/{id}/suspend` - Suspend organization
- `PATCH /platform/organizations/{id}/resume` - Resume organization
- `POST /platform/organizations/{id}/deprovision` - Deprovision organization
- `POST /platform/organizations/{id}/provision/retry` - Retry provisioning

## Architecture

### Frontend Structure

```
src/
├── components/
│   ├── CreateTenantModal.tsx    # Modal for creating new tenants
│   └── TenantList.tsx           # Table component for displaying tenants
├── contexts/
│   └── PlatformAuthContext.tsx # Authentication state management
├── lib/
│   ├── api.ts                   # Axios client configuration
│   └── platform-api.ts         # API service functions
├── pages/
│   ├── PlatformLogin.tsx        # Login page
│   └── PlatformDashboard.tsx    # Main dashboard
├── routes/
│   └── PlatformRoutes.tsx       # Route configuration with auth guards
└── App.tsx                      # Root component with router
```

### Key Features

1. **Authentication Context**: Manages platform admin session state using React Context
2. **Protected Routes**: Route guards to ensure only authenticated admins can access the dashboard
3. **API Client**: Axios instance with automatic token injection and 401 handling
4. **Type Safety**: TypeScript interfaces for all API requests/responses

## Development

### Building for Production

```bash
npm run build
```

### Linting

```bash
npm run lint
```

## React + TypeScript + Vite

This project uses React with TypeScript and Vite for fast development.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Oxc](https://oxc.rs)
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/)

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the Oxlint configuration

If you are developing a production application, we recommend enabling type-aware lint rules by installing `oxlint-tsgolint` and editing `.oxlintrc.json`:

```json
{
  "$schema": "./node_modules/oxlint/configuration_schema.json",
  "plugins": ["react", "typescript", "oxc"],
  "options": {
    "typeAware": true
  },
  "rules": {
    "react/rules-of-hooks": "error",
    "react/only-export-components": ["warn", { "allowConstantExport": true }]
  }
}
```

See the [Oxlint rules documentation](https://oxc.rs/docs/guide/usage/linter/rules) for the full list of rules and categories.

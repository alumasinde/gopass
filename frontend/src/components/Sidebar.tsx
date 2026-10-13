import { useLocation, useNavigate } from "react-router-dom";

interface NavItem {
  path: string;
  label: string;
  icon: string;
}

const navItems: NavItem[] = [
  { path: "/platform/dashboard", label: "Dashboard", icon: "📊" },
  { path: "/platform/tenants", label: "Tenants", icon: "🏢" },
  { path: "/platform/admins", label: "Admins", icon: "👤" },
  { path: "/platform/migrations", label: "Migrations", icon: "🔄" },
];

export default function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();

  return (
    <aside
      style={{
        width: "250px",
        backgroundColor: "#1f2937",
        color: "white",
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <div
        style={{
          padding: "1.5rem",
          borderBottom: "1px solid #374151",
        }}
      >
        <h2 style={{ fontSize: "1.25rem", fontWeight: "bold", margin: 0 }}>
          GoPass
        </h2>
        <p
          style={{
            fontSize: "0.75rem",
            color: "#9ca3af",
            margin: "0.25rem 0 0 0",
          }}
        >
          Platform Admin
        </p>
      </div>

      <nav style={{ flex: 1, padding: "1rem 0" }}>
        {navItems.map((item) => {
          const isActive =
            location.pathname === item.path ||
            (item.path !== "/platform/dashboard" &&
              location.pathname.startsWith(item.path));

          return (
            <button
              key={item.path}
              onClick={() => navigate(item.path)}
              style={{
                width: "100%",
                padding: "0.75rem 1.5rem",
                backgroundColor: isActive ? "#374151" : "transparent",
                color: isActive ? "white" : "#9ca3af",
                border: "none",
                textAlign: "left",
                fontSize: "0.875rem",
                cursor: "pointer",
                display: "flex",
                alignItems: "center",
                gap: "0.75rem",
                transition: "background-color 0.2s",
              }}
              onMouseEnter={(e) => {
                if (!isActive) {
                  e.currentTarget.style.backgroundColor = "#374151";
                }
              }}
              onMouseLeave={(e) => {
                if (!isActive) {
                  e.currentTarget.style.backgroundColor = "transparent";
                }
              }}
            >
              <span style={{ fontSize: "1.25rem" }}>{item.icon}</span>
              {item.label}
            </button>
          );
        })}
      </nav>

      <div style={{ padding: "1rem", borderTop: "1px solid #374151" }}>
        <button
          onClick={() => {
            localStorage.removeItem("platform_access_token");
            window.location.href = "/platform/login";
          }}
          style={{
            width: "100%",
            padding: "0.75rem",
            backgroundColor: "#dc2626",
            color: "white",
            border: "none",
            borderRadius: "4px",
            fontSize: "0.875rem",
            fontWeight: "500",
            cursor: "pointer",
          }}
        >
          Logout
        </button>
      </div>
    </aside>
  );
}

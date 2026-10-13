import { type ReactNode } from 'react';
import Sidebar from './Sidebar';

interface PlatformLayoutProps {
  children: ReactNode;
}

export default function PlatformLayout({ children }: PlatformLayoutProps) {
  return (
    <div className="platform-shell">
      <Sidebar />
      <main className="platform-main">{children}</main>
    </div>
  );
}

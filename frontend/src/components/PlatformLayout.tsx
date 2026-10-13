import { type ReactNode } from 'react';
import Sidebar from './Sidebar';

interface PlatformLayoutProps {
  children: ReactNode;
}

export default function PlatformLayout({ children }: PlatformLayoutProps) {
  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <Sidebar />
      <main style={{ flex: 1, backgroundColor: '#f9fafb', overflow: 'auto' }}>
        {children}
      </main>
    </div>
  );
}

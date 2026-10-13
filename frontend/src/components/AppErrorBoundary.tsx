import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';

interface Props { children: ReactNode }
interface State { error: Error | null }

export default class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State { return { error }; }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('GoPass UI rendering error', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <main className="app-fatal-error" role="alert">
          <div className="app-fatal-card">
            <span className="tenant-brand-mark">G</span>
            <p className="eyebrow">GOPASS WORKSPACE</p>
            <h1>This page could not be displayed</h1>
            <p>The application hit a rendering error instead of showing a blank screen. Refresh the page; if it continues, open the browser console and share the error details.</p>
            <details><summary>Technical details</summary><pre>{this.state.error.message}</pre></details>
            <button className="tenant-primary-button" onClick={() => window.location.reload()}>Refresh application</button>
          </div>
        </main>
      );
    }
    return this.props.children;
  }
}

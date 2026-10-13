/**
 * Tenant host resolution for local subdomains and configured platform domains.
 * Local development: http://<slug>.localhost:5173
 */
export function tenantSlugFromHostname(hostname: string): string {
  const host = hostname.trim().toLowerCase().split(':')[0].replace(/\.$/, '');
  if (host === 'localhost' || host === '127.0.0.1' || host === '::1') return '';
  if (host.endsWith('.localhost')) {
    const slug = host.slice(0, -'.localhost'.length);
    return /^[a-z0-9](?:[a-z0-9-]{0,98}[a-z0-9])?$/.test(slug) ? slug : '';
  }
  const baseDomain = (import.meta.env.VITE_TENANT_BASE_DOMAIN || '').trim().toLowerCase().replace(/^\./, '');
  if (baseDomain && host.endsWith(`.${baseDomain}`)) {
    const slug = host.slice(0, -(baseDomain.length + 1));
    return /^[a-z0-9](?:[a-z0-9-]{0,98}[a-z0-9])?$/.test(slug) ? slug : '';
  }
  return '';
}

export function isTenantHostname(hostname: string): boolean {
  const host = hostname.trim().toLowerCase().split(':')[0];
  return Boolean(tenantSlugFromHostname(host) || (host !== 'localhost' && host !== '127.0.0.1' && host !== '::1'));
}

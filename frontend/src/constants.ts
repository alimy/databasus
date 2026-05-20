interface RuntimeConfig {
  IS_CLOUD?: string;
  IS_DISABLE_CLOUD_NOTICE?: string;
  GITHUB_CLIENT_ID?: string;
  GOOGLE_CLIENT_ID?: string;
  IS_EMAIL_CONFIGURED?: string;
  CLOUDFLARE_TURNSTILE_SITE_KEY?: string;
  CONTAINER_ARCH?: string;
  CLOUD_PRICE_PER_GB?: string;
  CLOUD_PADDLE_CLIENT_TOKEN?: string;
  CLOUD_IS_PADDLE_SANDBOX?: string;
}

declare global {
  interface Window {
    __RUNTIME_CONFIG__?: RuntimeConfig;
  }
}

export function getApplicationServer() {
  const origin = window.location.origin;
  const url = new URL(origin);

  const isDevelopment = import.meta.env.MODE === 'development';

  if (isDevelopment) {
    return `${url.protocol}//${url.hostname}:4005`;
  } else {
    return `${url.protocol}//${url.hostname}:${url.port || (url.protocol === 'https:' ? '443' : '80')}`;
  }
}

export const APP_VERSION = (import.meta.env.VITE_APP_VERSION as string) || 'dev';

export const IS_CLOUD =
  window.__RUNTIME_CONFIG__?.IS_CLOUD === 'true' || import.meta.env.VITE_IS_CLOUD === 'true';

export const IS_DISABLE_CLOUD_NOTICE =
  window.__RUNTIME_CONFIG__?.IS_DISABLE_CLOUD_NOTICE === 'true' ||
  import.meta.env.VITE_IS_DISABLE_CLOUD_NOTICE === 'true';

export const CLOUD_PRICE_PER_GB_CENTS = Number(
  window.__RUNTIME_CONFIG__?.CLOUD_PRICE_PER_GB || import.meta.env.VITE_CLOUD_PRICE_PER_GB || '0',
);

export const GITHUB_CLIENT_ID =
  window.__RUNTIME_CONFIG__?.GITHUB_CLIENT_ID || import.meta.env.VITE_GITHUB_CLIENT_ID || '';

export const GOOGLE_CLIENT_ID =
  window.__RUNTIME_CONFIG__?.GOOGLE_CLIENT_ID || import.meta.env.VITE_GOOGLE_CLIENT_ID || '';

export const IS_EMAIL_CONFIGURED =
  window.__RUNTIME_CONFIG__?.IS_EMAIL_CONFIGURED === 'true' ||
  import.meta.env.VITE_IS_EMAIL_CONFIGURED === 'true';

export const CLOUDFLARE_TURNSTILE_SITE_KEY =
  window.__RUNTIME_CONFIG__?.CLOUDFLARE_TURNSTILE_SITE_KEY ||
  import.meta.env.VITE_CLOUDFLARE_TURNSTILE_SITE_KEY ||
  '';

export const PADDLE_CLIENT_TOKEN =
  window.__RUNTIME_CONFIG__?.CLOUD_PADDLE_CLIENT_TOKEN ||
  import.meta.env.VITE_CLOUD_PADDLE_CLIENT_TOKEN ||
  '';

export const IS_PADDLE_SANDBOX =
  window.__RUNTIME_CONFIG__?.CLOUD_IS_PADDLE_SANDBOX === 'true' ||
  import.meta.env.VITE_CLOUD_IS_PADDLE_SANDBOX === 'true';

const archMap: Record<string, string> = { amd64: 'x64', arm64: 'arm64' };
const rawArch = window.__RUNTIME_CONFIG__?.CONTAINER_ARCH || 'unknown';
export const CONTAINER_ARCH = archMap[rawArch] || rawArch;

export function getOAuthRedirectUri(): string {
  return `${window.location.origin}/auth/callback`;
}

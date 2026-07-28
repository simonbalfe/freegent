type BrowserProxy = {
  server: string;
  username: string;
  password: string;
};

export type ProxySession = {
  browserProxy: BrowserProxy;
  solverProxy: string;
};

const proxyConfig = {
  username: process.env.EVOMI_USERNAME ?? "",
  password: process.env.EVOMI_PASSWORD ?? "",
  gateway: process.env.EVOMI_GATEWAY ?? "",
};

export const hasProxy = Boolean(proxyConfig.username && proxyConfig.password && proxyConfig.gateway);

export function createProxySession(): ProxySession {
  const sessionID = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e6).toString(36)}`;
  const password = `${proxyConfig.password}_session-${sessionID}`;
  return {
    browserProxy: {
      server: `http://${proxyConfig.gateway}`,
      username: proxyConfig.username,
      password,
    },
    solverProxy: `${proxyConfig.gateway}:${proxyConfig.username}:${password}`,
  };
}

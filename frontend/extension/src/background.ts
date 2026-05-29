import { Storage } from "@plasmohq/storage";

const storage = new Storage();
const urlRegex = /https?:\/\/s\/(.+)/;

const isAllowedHost = (hostname: string, domain: string) =>
  hostname === domain || hostname.endsWith(`.${domain}`);

chrome.webRequest.onBeforeRequest.addListener(
  (param) => {
    (async () => {
      if (!param.url) {
        return;
      }

      const shortcutName = getShortcutNameFromUrl(param.url);
      if (shortcutName) {
        const instanceUrl = (await storage.getItem<string>("instance_url")) || "";
        const url = new URL(`/s/${shortcutName}`, instanceUrl);
        return chrome.tabs.update({ url: url.toString() });
      }
    })();
  },
  { urls: ["*://s/*", "*://*/search*", "*://*/s*", "*://duckduckgo.com/*"] },
);

const getShortcutNameFromUrl = (urlString: string) => {
  const matchResult = urlRegex.exec(urlString);
  if (matchResult === null) {
    return getShortcutNameFromSearchUrl(urlString);
  }
  return matchResult[1];
};

const getShortcutNameFromSearchUrl = (urlString: string) => {
  const url = new URL(urlString);
  if ((isAllowedHost(url.hostname, "google.com") || isAllowedHost(url.hostname, "bing.com")) && url.pathname === "/search") {
    const params = new URLSearchParams(url.search);
    const shortcutName = params.get("q");
    if (typeof shortcutName === "string" && shortcutName.startsWith("s/")) {
      return shortcutName.slice(2);
    }
  } else if (isAllowedHost(url.hostname, "baidu.com") && url.pathname === "/s") {
    const params = new URLSearchParams(url.search);
    const shortcutName = params.get("wd");
    if (typeof shortcutName === "string" && shortcutName.startsWith("s/")) {
      return shortcutName.slice(2);
    }
  } else if (isAllowedHost(url.hostname, "duckduckgo.com") && url.pathname === "/") {
    const params = new URLSearchParams(url.search);
    const shortcutName = params.get("q");
    if (typeof shortcutName === "string" && shortcutName.startsWith("s/")) {
      return shortcutName.slice(2);
    }
  }
  return "";
};

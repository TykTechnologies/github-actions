// src/index.ts
function sanitizeUrl(raw) {
  const abort = () => {
    throw new Error(`Invalid url to pass to open(): ${raw}`);
  };
  let url;
  try {
    url = new URL(raw);
  } catch (_) {
    abort();
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") abort();
  if (url.hostname !== encodeURIComponent(url.hostname)) abort();
  if (url.username) url.username = encodeURIComponent(url.username);
  if (url.password) url.password = encodeURIComponent(url.password);
  url.pathname = url.pathname.slice(0, 1) + encodeURIComponent(url.pathname.slice(1)).replace(/%2f/ig, "/");
  url.search = url.search.slice(0, 1) + Array.from(url.searchParams.entries()).map(sanitizeParam).join("&");
  url.hash = url.hash.slice(0, 1) + encodeURIComponent(url.hash.slice(1));
  return url.href;
}
function sanitizeParam([k, v]) {
  return `${encodeURIComponent(k)}${v.length > 0 ? `=${encodeURIComponent(v)}` : ""}`;
}
export {
  sanitizeUrl
};

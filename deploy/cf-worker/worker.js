// Minimal pass-through forwarder for HappySorter's proxy_url setting
// (docs/DEPLOYMENT.md § 4a). Deploy as a Cloudflare Worker, then paste its
// https://<name>.<subdomain>.workers.dev URL into Setup → Sources → Proxy
// URL. HappySorter calls it as `<worker-url>/?url=<encoded target>`; the
// Worker fetches that target with Cloudflare's own egress IP/edge and
// returns the response body and status untouched, which is enough to get
// past a Cloudflare challenge triggered by the NAS's own IP.
export default {
  async fetch(request) {
    const target = new URL(request.url).searchParams.get("url");
    if (!target) {
      return new Response("missing ?url= parameter", { status: 400 });
    }

    const upstream = await fetch(target, {
      method: request.method,
      headers: {
        "User-Agent":
          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36",
      },
      redirect: "manual",
    });

    return new Response(upstream.body, {
      status: upstream.status,
      headers: upstream.headers,
    });
  },
};

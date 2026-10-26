#!/usr/bin/env node
'use strict';
const http = require('node:http');
const https = require('node:https');
const MAX_BYTES = 1024 * 1024;
const DEFAULT_URL = 'http://127.0.0.1:20128/v1/models?prefix=alias';
const BASE_INSTRUCTIONS = "You are a coding assistant running in Codex CLI. Follow the user's instructions, use the available tools when appropriate, and report changes and tests accurately.";
function convertCatalog(payload) {
  if (!payload || !Array.isArray(payload.data)) throw new Error('expected OpenAI models object with a data array');
  const models = [], seen = new Set();
  for (const entry of payload.data) {
    if (!entry || typeof entry.id !== 'string' || !entry.id.trim() || seen.has(entry.id)) continue;
    seen.add(entry.id);
    const model = {
      slug: entry.id, display_name: entry.name || entry.display_name || entry.id,
      description: null, base_instructions: BASE_INSTRUCTIONS,
      model_messages: { instructions_template: BASE_INSTRUCTIONS },
      default_reasoning_level: null, supported_reasoning_levels: [],
      shell_type: 'shell_command', visibility: 'list', supported_in_api: true,
      priority: models.length + 1, availability_nux: null, upgrade: null,
      support_verbosity: false, default_verbosity: null, apply_patch_tool_type: null,
      truncation_policy: { mode: 'tokens', limit: 10000 },
      experimental_supported_tools: [], input_modalities: ['text']
    };
    const context = entry.context_window || entry.context_length;
    if (Number.isSafeInteger(context) && context > 0) model.context_window = context;
    models.push(model);
  }
  if (!models.length) throw new Error('OmniRoute returned no usable models');
  return { models };
}
function fetchUpstream(url, headers) {
  return new Promise((resolve, reject) => {
    let target;
    try { target = new URL(url); } catch { return reject(new Error('invalid OMNIROUTE_MODELS_URL')); }
    const client = target.protocol === 'https:' ? https : target.protocol === 'http:' ? http : null;
    if (!client) return reject(new Error('OMNIROUTE_MODELS_URL must use http or https'));
    const req = client.get(target, { headers }, res => {
      const status = res.statusCode || 502;
      if (status !== 200) {
        res.resume();
        return reject(Object.assign(new Error('OmniRoute returned HTTP ' + status), { upstreamStatus: status }));
      }
      let size = 0; const chunks = [];
      res.on('data', chunk => {
        size += chunk.length;
        if (size > MAX_BYTES) { reject(new Error('upstream catalog exceeds 1 MiB')); res.destroy(); }
        else chunks.push(chunk);
      });
      res.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
      res.on('error', reject);
    });
    req.setTimeout(10000, () => req.destroy(new Error('upstream timeout')));
    req.on('error', reject);
  });
}
function sendJson(res, status, data) {
  let body = Buffer.from(JSON.stringify(data));
  if (body.length > MAX_BYTES) {
    status = 502; body = Buffer.from('{"error":{"message":"model catalog exceeds 1 MiB"}}');
  }
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': body.length, 'Cache-Control': 'no-store' });
  res.end(body);
}
async function handleRequest(req, res) {
  let path;
  try { path = new URL(req.url, 'http://localhost').pathname; }
  catch { return sendJson(res, 400, { error: { message: 'invalid request URL' } }); }
  if (req.method !== 'GET' || path !== '/models') return sendJson(res, 404, { error: { message: 'not found' } });
  const headers = { Accept: 'application/json' };
  const incoming = req.headers.authorization || '';
  if (process.env.OMNIROUTE_MODELS_API_KEY) headers.Authorization = 'Bearer ' + process.env.OMNIROUTE_MODELS_API_KEY;
  else if (/^Bearer\s+\S+/i.test(incoming)) headers.Authorization = incoming;
  else if (process.env.OMNIROUTE_API_KEY) headers.Authorization = 'Bearer ' + process.env.OMNIROUTE_API_KEY;
  try {
    const catalog = convertCatalog(JSON.parse(await fetchUpstream(process.env.OMNIROUTE_MODELS_URL || DEFAULT_URL, headers)));
    console.log('translated ' + catalog.models.length + ' OmniRoute models');
    sendJson(res, 200, catalog);
  } catch (err) {
    if (err.upstreamStatus) {
      const status = [401,403,429].includes(err.upstreamStatus) ? err.upstreamStatus : 502;
      sendJson(res, status, { error: { message: err.message } });
    } else {
      console.error('catalog error: ' + err.message);
      sendJson(res, 502, { error: { message: 'invalid or unavailable OmniRoute catalog' } });
    }
  }
}
if (require.main === module) {
  const host = process.env.HOST || '127.0.0.1', port = Number(process.env.PORT || 8787);
  http.createServer(handleRequest).listen(port, host, () => console.log('Codex catalog adapter listening on http://' + host + ':' + port + '/models'));
}
module.exports = { convertCatalog, handleRequest };

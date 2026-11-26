// Acesso à API local. Toda alteração leva o cabeçalho que o daemon exige.

export class ErroDaApi extends Error {
  constructor(
    mensagem: string,
    readonly codigo: number,
  ) {
    super(mensagem);
  }
}

async function pedir<T>(metodo: string, rota: string, corpo?: unknown): Promise<T> {
  const opcoes: RequestInit = { method: metodo, headers: {} };
  const cabecalhos = opcoes.headers as Record<string, string>;
  if (metodo !== "GET") cabecalhos["X-Palantir"] = "1";
  if (corpo instanceof FormData) {
    opcoes.body = corpo;
  } else if (corpo !== undefined) {
    cabecalhos["Content-Type"] = "application/json";
    opcoes.body = JSON.stringify(corpo);
  }
  let resposta: Response;
  try {
    resposta = await fetch(rota, opcoes);
  } catch {
    throw new ErroDaApi("O daemon não está respondendo.", 0);
  }
  const dados = await resposta.json().catch(() => null);
  if (!resposta.ok) {
    throw new ErroDaApi(dados?.erro ?? `Falha HTTP ${resposta.status}`, resposta.status);
  }
  return dados as T;
}

export const api = {
  obter: <T>(rota: string) => pedir<T>("GET", rota),
  enviar: <T>(rota: string, corpo?: unknown) => pedir<T>("POST", rota, corpo ?? {}),
  gravar: <T>(rota: string, corpo: unknown) => pedir<T>("PUT", rota, corpo),
  apagar: <T>(rota: string) => pedir<T>("DELETE", rota),
  /** Executa uma ação de atividade e devolve a resposta dela. */
  async agir<T = null>(atividade: number, acao: string, corpo: unknown = {}): Promise<T> {
    const r = await pedir<{ resposta: T }>("POST", `/api/atividades/${atividade}/acoes/${acao}`, corpo);
    return r.resposta;
  },
};

export interface Aviso {
  tipo: "atividade" | "acervo" | "ajustes" | "tudo" | "conexao";
  atividade?: number;
}

type Ouvinte = (a: Aviso) => void;

const ouvintes = new Set<Ouvinte>();

export function ouvir(o: Ouvinte): () => void {
  ouvintes.add(o);
  return () => ouvintes.delete(o);
}

function publicar(a: Aviso) {
  for (const o of ouvintes) o(a);
}

export let conectado = false;

/** Mantém o WebSocket de avisos, reconectando sozinho. */
export function conectar(espera = 500) {
  const ws = new WebSocket(`ws://${location.host}/api/eventos`);
  ws.onopen = () => {
    conectado = true;
    espera = 500;
    publicar({ tipo: "conexao" });
    // O que mudou enquanto estava desconectado é buscado de novo.
    publicar({ tipo: "tudo" });
  };
  ws.onmessage = (m) => publicar(JSON.parse(m.data) as Aviso);
  ws.onclose = () => {
    conectado = false;
    publicar({ tipo: "conexao" });
    setTimeout(() => conectar(Math.min(espera * 2, 10_000)), espera);
  };
}

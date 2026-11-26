import { LitElement } from "lit";
import { comBilhete, ouvir, type Aviso } from "./api";

/** Componente que desenha no DOM comum e usa a folha de estilo global. */
export class Elemento extends LitElement {
  override createRenderRoot() {
    return this;
  }

  /** Mostra uma mensagem passageira. */
  avisar(texto: string, erro = false) {
    this.dispatchEvent(new CustomEvent("pl-avisar", { detail: { texto, erro }, bubbles: true }));
  }

  /** Executa uma operação, mostrando o erro e evitando cliques repetidos. */
  async tentar(f: () => Promise<unknown>, sucesso?: string): Promise<boolean> {
    try {
      await f();
      if (sucesso) this.avisar(sucesso);
      return true;
    } catch (e) {
      this.avisar(e instanceof Error ? e.message : String(e), true);
      return false;
    }
  }
}

/**
 * Componente que busca dados no daemon e os atualiza quando um aviso chega.
 * Pedidos concorrentes são agrupados, e uma resposta antiga nunca substitui uma
 * mais nova.
 */
export abstract class Visao<T> extends Elemento {
  static override properties = { dados: { state: true }, falha: { state: true } };

  declare dados: T | undefined;
  declare falha: string;

  private deixarDeOuvir?: () => void;
  private buscando = false;
  private denovo = false;
  private geracao = 0;

  constructor() {
    super();
    this.falha = "";
  }

  protected abstract buscar(): Promise<T>;

  protected interessa(_a: Aviso): boolean {
    return true;
  }

  override connectedCallback() {
    super.connectedCallback();
    this.deixarDeOuvir = ouvir((a) => {
      if (a.tipo === "tudo" || (a.tipo !== "conexao" && this.interessa(a))) void this.recarregar();
    });
    void this.recarregar();
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    this.deixarDeOuvir?.();
  }

  /** Descarta o que está na tela e busca do zero. */
  protected reiniciar() {
    this.geracao++;
    this.dados = undefined;
    this.falha = "";
    this.buscando = false;
    void this.recarregar();
  }

  async recarregar(): Promise<void> {
    if (this.buscando) {
      this.denovo = true;
      return;
    }
    this.buscando = true;
    const geracao = this.geracao;
    try {
      const dados = await this.buscar();
      if (geracao !== this.geracao) return;
      this.dados = dados;
      this.falha = "";
    } catch (e) {
      if (geracao !== this.geracao) return;
      this.falha = e instanceof Error ? e.message : String(e);
    } finally {
      if (geracao === this.geracao) {
        this.buscando = false;
        if (this.denovo) {
          this.denovo = false;
          void this.recarregar();
        }
      }
    }
  }
}

const dinheiro = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 4 });

export const usd = (v: number) => dinheiro.format(v);

export const quando = (iso: string) =>
  new Date(iso).toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });

export function tamanho(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1).replace(".", ",")} MB`;
}

export const nomeDoEstado: Record<string, string> = {
  rodando: "rodando",
  esperando_voce: "esperando você",
  pronta: "pronta",
  pausada: "pausada",
  arquivada: "arquivada",
};

export const nomeDoTipo: Record<string, string> = {
  conversa: "Conversa",
  relatorio: "Relatório",
  triagem: "Triagem de e-mail",
  estudo: "Estudo para concurso",
};

export const nomeDaClasse: Record<string, string> = {
  "": "sem classe",
  urgente: "urgente",
  responder: "responder",
  ler_depois: "ler depois",
  lixo: "lixo",
};

/** Informa se a tecla foi digitada num campo, onde atalhos não valem. */
export function digitando(e: KeyboardEvent) {
  const alvo = e.target as HTMLElement | null;
  return !!alvo?.closest("input, textarea, select, [contenteditable]") || e.ctrlKey || e.metaKey || e.altKey;
}

/** Importa arquivos escolhidos ou arrastados. */
export function formularioDeArquivos(arquivos: Iterable<File>) {
  const f = new FormData();
  for (const a of arquivos) f.append("arquivo", a, a.name);
  return f;
}

/** Baixa um arquivo da API, com um endereço de uso temporário. */
export async function baixar(caminho: string) {
  const a = document.createElement("a");
  a.href = await comBilhete(caminho);
  a.download = "";
  a.click();
}

// Extensões de arquivo que parecem um domínio.
const arquivos = /\.(pdf|txt|docx?|xlsx?|pptx?|od[tsp]|csv|zip|png|jpe?g|eml|html?|md)$/i;

/**
 * Devolve o endereço quando o texto é só um endereço web, e não um pedido.
 * Apenas http e https.
 */
export function soEndereco(texto: string): string | undefined {
  const t = texto.trim();
  if (!t || /\s/.test(t)) return undefined;
  const comEsquema = /^https?:\/\//i.test(t);
  const semEsquema = /^(localhost(:\d+)?|([\p{L}\d-]+\.)+\p{L}{2,}(:\d+)?)([/?#].*)?$/iu.test(t);
  if (!comEsquema && (!semEsquema || (!t.includes("/") && arquivos.test(t) && !/^www\./i.test(t)))) return undefined;
  try {
    return new URL(comEsquema ? t : `${t.startsWith("localhost") ? "http" : "https"}://${t}`).href;
  } catch {
    return undefined;
  }
}

/** Abre um site noutra aba; o Palantír continua nesta. */
export function navegar(endereco: string) {
  window.open(endereco, "_blank", "noopener,noreferrer");
}

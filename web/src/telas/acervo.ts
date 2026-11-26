import { html, nothing, type PropertyValues } from "lit";
import { api, comBilhete, type Aviso } from "../api";
import { Visao, formularioDeArquivos, quando, tamanho, baixar } from "../base";
import type { Colecao, Documento, Entidade, Trecho } from "../tipos";

interface Lista {
  documentos: Documento[];
  numeros: { documentos: number; pendentes: number; com_erro: number; bytes: number };
  pasta_entrada: string;
  colecoes: Colecao[];
  entidades: Entidade[];
}

interface Aberto {
  documento: Documento;
  trechos: Trecho[];
  entidades: Entidade[];
  colecoes: Colecao[];
}

interface Relatorio {
  importados: number;
  repetidos: number;
  falhas: string[];
}

export function resumoDaImportacao(r: Relatorio) {
  let t = `${r.importados} importado(s)`;
  if (r.repetidos) t += `, ${r.repetidos} já no acervo`;
  if (r.falhas.length) t += `. Falhas: ${r.falhas.join("; ")}`;
  return t;
}

/** Importar, procurar e organizar documentos, sem pastas. */
export class PlAcervo extends Visao<Lista> {
  static override properties = {
    ...Visao.properties,
    colecao: { state: true },
    entidade: { state: true },
    achados: { state: true },
    consulta: { state: true },
    trabalhando: { state: true },
  };

  declare colecao: number;
  declare entidade: string;
  declare achados: Trecho[] | undefined;
  declare consulta: string;
  declare trabalhando: boolean;

  constructor() {
    super();
    this.colecao = 0;
    this.entidade = "";
    this.consulta = "";
    this.trabalhando = false;
  }

  protected override async buscar() {
    const q = new URLSearchParams();
    if (this.colecao) q.set("colecao", String(this.colecao));
    if (this.entidade) q.set("entidade", this.entidade);
    const [lista, visoes] = await Promise.all([
      api.obter<Omit<Lista, "colecoes" | "entidades">>(`/api/acervo/documentos?${q}`),
      api.obter<{ colecoes: Colecao[]; entidades: Entidade[] }>("/api/acervo/visoes"),
    ]);
    return { ...lista, ...visoes };
  }

  protected override interessa(a: Aviso) {
    return a.tipo === "acervo" || a.tipo === "ajustes";
  }

  private async ocupar(f: () => Promise<unknown>) {
    this.trabalhando = true;
    await this.tentar(f);
    this.trabalhando = false;
    await this.recarregar();
  }

  private async enviar(e: Event) {
    const campo = e.target as HTMLInputElement;
    if (!campo.files?.length) return;
    await this.ocupar(async () => {
      const r = await api.enviar<{ relatorio: Relatorio }>("/api/acervo/importar", formularioDeArquivos(campo.files!));
      this.avisar(resumoDaImportacao(r.relatorio), r.relatorio.falhas.length > 0);
    });
    campo.value = "";
  }

  private async pasta(e: SubmitEvent) {
    e.preventDefault();
    const formulario = e.target as HTMLFormElement;
    const caminho = String(new FormData(formulario).get("caminho") ?? "");
    await this.ocupar(async () => {
      const r = await api.enviar<{ relatorio: Relatorio }>("/api/acervo/importar-pasta", { caminho });
      this.avisar(resumoDaImportacao(r.relatorio), r.relatorio.falhas.length > 0);
      formulario.reset();
    });
  }

  private async procurar(e: SubmitEvent) {
    e.preventDefault();
    const q = String(new FormData(e.target as HTMLFormElement).get("q") ?? "").trim();
    this.consulta = q;
    if (!q) {
      this.achados = undefined;
      return;
    }
    await this.tentar(async () => {
      this.achados = (await api.obter<{ achados: Trecho[] }>(`/api/acervo/busca?q=${encodeURIComponent(q)}`)).achados;
    });
  }

  private async novaColecao(e: SubmitEvent) {
    e.preventDefault();
    const formulario = e.target as HTMLFormElement;
    const nome = String(new FormData(formulario).get("nome") ?? "");
    await this.ocupar(async () => {
      await api.enviar("/api/acervo/colecoes", { nome });
      formulario.reset();
    });
  }

  private manutencao(rota: string) {
    return this.ocupar(async () => {
      const r = await api.enviar<{ conferidos: number; problemas: { nome: string; motivo: string }[] }>(rota);
      this.avisar(
        r.problemas.length
          ? `Problemas: ${r.problemas.map((p) => `${p.nome}: ${p.motivo}`).join("; ")}`
          : `${r.conferidos} original(is) íntegro(s).`,
        r.problemas.length > 0,
      );
    });
  }

  private filtrar(colecao: number, entidade: string) {
    this.colecao = colecao;
    this.entidade = entidade;
    this.achados = undefined;
    void this.recarregar();
  }

  private lista(d: Lista) {
    if (this.achados) {
      return html`<h2>Resultados para “${this.consulta}”</h2>
        <button type="button" class="discreto" @click=${() => (this.achados = undefined)}>Limpar busca</button>
        ${this.achados.length === 0 ? html`<p class="vazio">Nada encontrado.</p>` : nothing}
        ${this.achados.map(
          (t) => html`<article class="cartao">
            <h3><a href="#/acervo/documento/${t.documento}">${t.nome}</a> <span class="suave pequeno">página ${t.pagina}</span></h3>
            <p class="pequeno">${t.texto.slice(0, 500)}</p>
          </article>`,
        )}`;
    }
    const filtro = this.colecao || this.entidade;
    return html`<h2>Documentos</h2>
      ${filtro ? html`<button type="button" class="discreto" @click=${() => this.filtrar(0, "")}>Mostrar todos</button>` : nothing}
      ${d.documentos.length === 0
        ? html`<p class="vazio">Nenhum documento. Arraste arquivos para a tela ou copie para ${d.pasta_entrada}.</p>`
        : html`<ul class="lista">
            ${d.documentos.map(
              (x) => html`<li>
                <a class="item" href="#/acervo/documento/${x.id}">
                  <span class="cresce">
                    <strong>${x.nome}</strong>
                    <span class="suave pequeno">${x.resumo || `${tamanho(x.tamanho)} · ${quando(x.importado_em)}`}${x.erro ? ` · ${x.erro}` : ""}</span>
                  </span>
                  <span class="selo ${x.estado}">${x.estado === "pendente" ? html`<i class="girando"></i> extraindo` : x.estado}</span>
                </a>
              </li>`,
            )}
          </ul>`}`;
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Carregando…"}</p>`;
    return html`
      <h1>Acervo</h1>
      <p class="suave pequeno">${d.numeros.documentos} documento(s), ${tamanho(d.numeros.bytes)}. Os originais nunca são alterados.</p>
      <form class="linha-flex" @submit=${this.procurar} role="search">
        <input class="cresce" type="search" name="q" aria-label="Procurar no acervo" placeholder="O que você procura?" style="width:auto" />
        <button type="submit" class="primario">Procurar</button>
      </form>
      <div class="duas">
        <div>${this.lista(d)}</div>
        <aside>
          <h2>Importar</h2>
          <div class="cartao">
            <label for="arquivos">Arquivos (ou arraste para a tela)</label>
            <input id="arquivos" type="file" multiple ?disabled=${this.trabalhando} @change=${this.enviar} />
            <form @submit=${this.pasta}>
              <label for="caminho">Pasta ou arquivo deste computador</label>
              <div class="linha-flex">
                <input id="caminho" class="cresce" name="caminho" placeholder="~/Documentos" required style="width:auto" />
                <button type="submit" ?disabled=${this.trabalhando}>Importar</button>
              </div>
            </form>
            <p class="pequeno suave">O que for copiado para ${d.pasta_entrada} entra sozinho.</p>
          </div>
          <h2>Coleções</h2>
          <div class="linha-flex">
            ${d.colecoes.map(
              (c) => html`<button type="button" aria-pressed=${this.colecao === c.id} @click=${() => this.filtrar(c.id, "")}>
                ${c.nome} (${c.documentos})${c.criada_por === "ia" ? html` <span class="suave pequeno">IA</span>` : nothing}
              </button>`,
            )}
          </div>
          <form class="linha-flex" @submit=${this.novaColecao} style="margin-top:0.5rem">
            <input class="cresce" name="nome" aria-label="Nome da nova coleção" placeholder="Nova coleção" required maxlength="60" style="width:auto" />
            <button type="submit">Criar</button>
          </form>
          ${d.entidades.length
            ? html`<h2>Entidades</h2>
                <div class="linha-flex">
                  ${d.entidades.slice(0, 30).map(
                    (e) => html`<button type="button" class="pequeno" title=${e.tipo} aria-pressed=${this.entidade === `${e.tipo}:${e.valor}`}
                      @click=${() => this.filtrar(0, `${e.tipo}:${e.valor}`)}>${e.valor}</button>`,
                  )}
                </div>`
            : nothing}
          <h2>Manutenção</h2>
          <div class="linha-flex">
            <button type="button" @click=${() => this.tentar(() => baixar("/api/exportar"), "Preparando o arquivo; ele vai para os downloads.")}>
              Baixar uma exportação
            </button>
            <button type="button" ?disabled=${this.trabalhando} @click=${() => this.manutencao("/api/acervo/verificar")}>Verificar originais</button>
            <button type="button" ?disabled=${this.trabalhando} @click=${() => this.manutencao("/api/acervo/regenerar")}>Regenerar derivados</button>
          </div>
        </aside>
      </div>
    `;
  }
}
customElements.define("pl-acervo", PlAcervo);

/** Um documento: texto extraído, entidades, coleções e o original. */
export class PlDocumento extends Visao<Aberto> {
  static override properties = { ...Visao.properties, numero: { type: Number }, original: { state: true } };

  declare numero: number;
  /** Endereço temporário do original exibido numa moldura da própria tela. */
  declare original: string;

  constructor() {
    super();
    this.original = "";
  }

  private async alternarOriginal() {
    if (this.original) {
      this.original = "";
      return;
    }
    const numero = this.numero;
    await this.tentar(async () => {
      const endereco = await comBilhete(`/api/acervo/documentos/${numero}/original`);
      if (numero === this.numero) this.original = endereco;
    });
  }

  protected override async buscar() {
    const [d, v] = await Promise.all([
      api.obter<Omit<Aberto, "colecoes">>(`/api/acervo/documentos/${this.numero}`),
      api.obter<{ colecoes: Colecao[] }>("/api/acervo/visoes"),
    ]);
    return { ...d, colecoes: v.colecoes };
  }

  protected override interessa(a: Aviso) {
    return a.tipo === "acervo";
  }

  override willUpdate(mudou: PropertyValues<this>) {
    if (mudou.has("numero") && mudou.get("numero") !== undefined) {
      this.original = "";
      this.reiniciar();
    }
  }

  /** O navegador só exibe estes tipos; os outros vão para a pasta de downloads. */
  private exibivel(tipo: string) {
    return ["application/pdf", "image/png", "image/jpeg", "message/rfc822"].includes(tipo) || /^text\/(plain|markdown|csv|html)$/.test(tipo);
  }

  private async incluir(e: SubmitEvent) {
    e.preventDefault();
    const id = Number(new FormData(e.target as HTMLFormElement).get("colecao"));
    if (!id) return;
    await this.tentar(
      () => api.enviar(`/api/acervo/colecoes/${id}`, { documento: this.numero, incluir: true }),
      "Incluído na coleção.",
    );
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Carregando…"}</p>`;
    const x = d.documento;
    return html`
      <p class="pequeno"><a href="#/acervo">← Acervo</a></p>
      <h1>${x.nome}</h1>
      <p class="suave pequeno">
        ${tamanho(x.tamanho)} · ${x.paginas} página(s) · ${x.origem} · ${quando(x.importado_em)} ·
        <span class="selo ${x.estado}">${x.estado}</span><br />SHA-256 ${x.sha256}
      </p>
      ${x.erro ? html`<div class="aviso">${x.erro}</div>` : nothing}
      ${x.resumo ? html`<p>${x.resumo}</p>` : nothing}
      <div class="linha-flex">
        ${this.exibivel(x.tipo)
          ? html`<button type="button" aria-pressed=${!!this.original} @click=${() => this.alternarOriginal()}>
              ${this.original ? "Fechar o original" : "Ver o original"}
            </button>`
          : html`<button type="button" @click=${() => this.tentar(() => baixar(`/api/acervo/documentos/${x.id}/original`))}>
              Baixar o original
            </button>`}
        <button type="button" @click=${() => this.tentar(() => api.enviar(`/api/acervo/documentos/${x.id}/reextrair`), "Extraindo novamente.")}>
          Extrair novamente
        </button>
        ${d.colecoes.length
          ? html`<form class="linha-flex" @submit=${this.incluir}>
              <select name="colecao" aria-label="Coleção" style="width:auto">
                ${d.colecoes.map((c) => html`<option value=${c.id}>${c.nome}</option>`)}
              </select>
              <button type="submit">Incluir na coleção</button>
            </form>`
          : nothing}
      </div>
      ${this.original
        ? html`<iframe class="original" title="Original de ${x.nome}" src=${this.original}></iframe>`
        : nothing}
      ${d.entidades.length
        ? html`<h2>Entidades</h2><div class="linha-flex">${d.entidades.map((e) => html`<span class="selo" title=${e.tipo}>${e.valor}</span>`)}</div>`
        : nothing}
      <h2>Texto extraído</h2>
      ${d.trechos.length === 0 ? html`<p class="vazio">Sem texto.</p>` : nothing}
      ${d.trechos.map(
        (t) => html`<article class="cartao" id="trecho-${t.id}">
          <p class="pequeno suave">Página ${t.pagina} · T${t.id}</p>
          <p style="white-space:pre-wrap;overflow-wrap:anywhere">${t.texto}</p>
        </article>`,
      )}
    `;
  }
}
customElements.define("pl-documento", PlDocumento);

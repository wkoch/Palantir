import { html, nothing } from "lit";
import { api, type Aviso } from "../api";
import { Visao, navegar, nomeDoEstado, nomeDoTipo, quando, soEndereco, usd } from "../base";
import type { Atividade, Documento, Estado } from "../tipos";

const tipos = [
  { id: "conversa", nome: "Conversa", dica: "Perguntar sobre os documentos ou uma página", exemplo: "O que eu tenho sobre o contrato de aluguel?" },
  { id: "relatorio", nome: "Relatório", dica: "Pesquisar e redigir com fontes", exemplo: "Resuma os prazos e valores do meu contrato de aluguel." },
  { id: "triagem", nome: "Triagem de e-mail", dica: "Ver o que importa na caixa de entrada", exemplo: "" },
  { id: "estudo", nome: "Estudo", dica: "Edital, questões e revisões", exemplo: "Cole aqui o conteúdo programático, uma disciplina por linha: Direito Administrativo: Atos; Licitações" },
];

/** A tela inicial: o que precisa de você, o que ficou pronto e o que anda. */
export class PlFila extends Visao<Estado> {
  static override properties = {
    ...Visao.properties,
    tipo: { state: true },
    arquivadas: { state: true },
    criando: { state: true },
    documentos: { state: true },
    endereco: { state: true },
  };

  declare tipo: string;
  declare arquivadas: boolean;
  declare criando: boolean;
  declare documentos: Documento[];
  /** Preenchido quando o texto digitado é só um endereço. */
  declare endereco: string | undefined;

  constructor() {
    super();
    this.tipo = "conversa";
    this.arquivadas = false;
    this.criando = false;
    this.documentos = [];
  }

  protected override buscar() {
    return api.obter<Estado>(`/api/estado${this.arquivadas ? "?arquivadas=1" : ""}`);
  }

  protected override interessa(a: Aviso) {
    return a.tipo === "atividade" || a.tipo === "ajustes";
  }

  private querFoco = false;

  /** Foca o campo de nova atividade, assim que ele existir. */
  focarNova() {
    this.querFoco = true;
    this.focarSeDer();
  }

  private focarSeDer() {
    if (!this.querFoco) return;
    const alvo = this.querySelector<HTMLElement>("[name=objetivo], .nova button[type=submit]");
    if (alvo) {
      alvo.focus();
      this.querFoco = false;
    }
  }

  override updated() {
    this.focarSeDer();
  }

  private aceitaEndereco() {
    return this.tipo === "conversa" || this.tipo === "relatorio";
  }

  private async escolher(tipo: string) {
    this.tipo = tipo;
    this.endereco = undefined;
    if (tipo === "estudo") {
      await this.tentar(async () => {
        const r = await api.obter<{ documentos: Documento[] }>("/api/acervo/documentos");
        this.documentos = r.documentos.filter((d) => d.estado === "pronto");
      });
    }
    await this.updateComplete;
    this.focarNova();
  }

  private async criar(e: SubmitEvent) {
    e.preventDefault();
    const formulario = e.target as HTMLFormElement;
    const f = new FormData(formulario);
    const endereco = this.aceitaEndereco() ? soEndereco(String(f.get("objetivo") ?? "")) : undefined;
    if (endereco) {
      formulario.reset();
      this.endereco = undefined;
      navegar(endereco);
      return;
    }
    const opcoes: Record<string, unknown> = {};
    if (this.tipo === "conversa") opcoes["forte"] = f.has("forte");
    if (this.tipo === "relatorio") opcoes["profundo"] = f.has("profundo");
    if (this.tipo === "estudo" && f.get("edital")) opcoes["edital_documento"] = Number(f.get("edital"));
    this.criando = true;
    await this.tentar(async () => {
      const x = await api.enviar<Atividade>("/api/atividades", {
        tipo: this.tipo,
        titulo: String(f.get("titulo") ?? ""),
        objetivo: String(f.get("objetivo") ?? ""),
        opcoes,
      });
      formulario.reset();
      location.hash = `#/atividade/${x.id}`;
    });
    this.criando = false;
  }

  private teclas(e: KeyboardEvent) {
    // Enter envia; Shift+Enter quebra a linha. No estudo, o texto é longo.
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing && this.tipo !== "estudo") {
      e.preventDefault();
      (e.target as HTMLTextAreaElement).form?.requestSubmit();
    }
  }

  private formulario() {
    const t = tipos.find((x) => x.id === this.tipo)!;
    return html`<form class="cartao nova" @submit=${this.criar} aria-label="Nova atividade">
      <div class="tipos" role="group" aria-label="Tipo de atividade">
        ${tipos.map(
          (x) => html`<button type="button" aria-pressed=${x.id === this.tipo} @click=${() => this.escolher(x.id)}>
            <strong>${x.nome}</strong><span class="suave pequeno">${x.dica}</span>
          </button>`,
        )}
      </div>
      ${this.tipo === "triagem"
        ? html`<p class="suave">Busca as mensagens novas, classifica e etiqueta. Não envia, não move e não apaga nada.</p>`
        : html`${this.tipo === "estudo"
              ? html`<label for="edital">Edital no acervo (opcional)</label>
                  <select id="edital" name="edital">
                    <option value="">Colar o conteúdo abaixo</option>
                    ${this.documentos.map((d) => html`<option value=${d.id}>${d.nome}</option>`)}
                  </select>
                  <label for="titulo">Título</label>
                  <input id="titulo" name="titulo" placeholder="Concurso de analista" />`
              : nothing}
            <label for="objetivo">${this.tipo === "estudo" ? "Conteúdo programático ou cargo" : "O que você quer fazer? Ou um endereço para visitar"}</label>
            <textarea id="objetivo" name="objetivo" rows="3" placeholder=${t.exemplo} @keydown=${this.teclas}
              @input=${(e: Event) => {
                this.endereco = this.aceitaEndereco() ? soEndereco((e.target as HTMLTextAreaElement).value) : undefined;
              }}></textarea>`}
      <div class="linha-flex entre">
        <div>
          ${this.tipo === "conversa"
            ? html`<label class="linha"><input type="checkbox" name="forte" /> Usar o modelo forte</label>`
            : this.tipo === "relatorio"
              ? html`<label class="linha"><input type="checkbox" name="profundo" /> Pesquisa profunda (modelo forte)</label>`
              : nothing}
        </div>
        <button type="submit" class="primario" ?disabled=${this.criando}>${this.endereco ? "Abrir o site" : "Começar"}</button>
      </div>
    </form>`;
  }

  private item(a: Atividade) {
    return html`<li>
      <a class="item" href="#/atividade/${a.id}">
        <span class="cresce">
          <strong>${a.titulo}</strong>
          <span class="suave pequeno">${nomeDoTipo[a.tipo] ?? a.tipo} · ${quando(a.atualizada_em)}${a.aviso ? ` · ${a.aviso}` : ""}</span>
        </span>
        <span class="suave pequeno" title="Custo desta atividade">${usd(a.custo_usd)}</span>
        <span class="selo ${a.estado}">${a.estado === "rodando" ? html`<i class="girando"></i> ` : nothing}${nomeDoEstado[a.estado]}</span>
      </a>
    </li>`;
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Carregando…"}</p>`;
    const grupo = (titulo: string, estado: string) => {
      const lista = d.atividades.filter((a) => a.estado === estado);
      return lista.length
        ? html`<section aria-label=${titulo}><h2>${titulo}</h2><ul class="lista">${lista.map((a) => this.item(a))}</ul></section>`
        : nothing;
    };
    const tituloDe = (id: number) => d.atividades.find((a) => a.id === id)?.titulo ?? `atividade ${id}`;
    return html`
      <h1 class="escondido">Fila de atenção</h1>
      ${d.modelos === "sem_chave"
        ? html`<div class="aviso">
            Sem chave do OpenRouter, só as funções locais respondem: acervo, busca e revisões.
            <a href="#/ajustes">Configurar a chave</a>.
          </div>`
        : nothing}
      ${this.formulario()}
      ${d.confirmacoes.length
        ? html`<section aria-label="Confirmações"><h2>Aguardando o seu clique</h2>
            ${d.confirmacoes.map((c) => html`<pl-confirmacao .c=${c} titulo=${tituloDe(c.atividade)}></pl-confirmacao>`)}
          </section>`
        : nothing}
      ${grupo("Precisa de você", "esperando_voce")} ${grupo("Pronto", "pronta")} ${grupo("Em andamento", "rodando")}
      ${grupo("Pausadas", "pausada")} ${this.arquivadas ? grupo("Arquivadas", "arquivada") : nothing}
      ${d.atividades.length === 0 ? html`<p class="vazio">Nenhuma atividade ainda. Descreva acima o que quer fazer.</p>` : nothing}
      <p class="linha-flex entre pequeno suave">
        <span>Gasto no mês: ${usd(d.gasto_mes_usd)} de ${usd(d.teto_mes_usd)}</span>
        <label class="linha">
          <input type="checkbox" .checked=${this.arquivadas}
            @change=${(e: Event) => { this.arquivadas = (e.target as HTMLInputElement).checked; void this.recarregar(); }} />
          Mostrar arquivadas
        </label>
      </p>
    `;
  }
}
customElements.define("pl-fila", PlFila);

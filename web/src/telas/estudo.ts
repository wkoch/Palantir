import { html, nothing } from "lit";
import { api } from "../api";
import { Elemento, digitando, quando } from "../base";
import type { Assunto, Correcao, Estudo, FonteEstudo } from "../tipos";
import { indicadores } from "./quadro";

const notas = ["Errei", "Difícil", "Bom", "Fácil"];

type Aba = "assuntos" | "cartoes" | "questoes" | "painel";

type Questao = Estudo["questoes"][number];

const porcento = (parte: number, todo: number) => (todo ? `${Math.round((100 * parte) / todo)}%` : "—");

/** Assuntos do edital, cartões, questionário e desempenho. */
export class PlEstudo extends Elemento {
  static override properties = {
    atividade: { type: Number },
    dados: { attribute: false },
    ocupada: { type: Boolean },
    aba: { state: true },
    virado: { state: true },
    questao: { state: true },
    escolha: { state: true },
    correcao: { state: true },
    revisando: { state: true },
  };

  declare atividade: number;
  declare dados: Estudo;
  declare ocupada: boolean;
  declare aba: Aba;
  declare virado: boolean;
  /** A questão em tela fica fixa até o usuário pedir a próxima. */
  declare questao: Questao | undefined;
  declare escolha: number;
  declare correcao: Correcao | undefined;
  declare revisando: boolean;

  constructor() {
    super();
    this.aba = "assuntos";
    this.virado = false;
    this.escolha = -1;
    this.revisando = false;
  }

  private tecla = (e: KeyboardEvent) => this.atalho(e);

  override connectedCallback() {
    super.connectedCallback();
    window.addEventListener("keydown", this.tecla);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener("keydown", this.tecla);
  }

  private atalho(e: KeyboardEvent) {
    if (digitando(e) || this.aba !== "cartoes" || !this.dados.cartoes.length) return;
    // O foco costuma ficar na aba recém-escolhida; ali o espaço também vira o cartão.
    if (e.key === " " && !(e.target as HTMLElement).closest("button:not([role=tab]), a, summary")) {
      e.preventDefault();
      this.virado = !this.virado;
    } else if (this.virado && ["1", "2", "3", "4"].includes(e.key)) {
      e.preventDefault();
      void this.revisar(Number(e.key));
    }
  }

  private async revisar(nota: number) {
    const cartao = this.dados.cartoes[0];
    if (!cartao || this.revisando) return;
    this.revisando = true;
    if (await this.tentar(() => api.agir(this.atividade, "revisar", { cartao: cartao.id, nota }))) {
      // Esconde o verso antes de o próximo cartão aparecer.
      this.virado = false;
      this.dados = { ...this.dados, cartoes: this.dados.cartoes.filter((c) => c.id !== cartao.id) };
    }
    this.revisando = false;
  }

  private async responder(q: Questao) {
    if (this.escolha < 0) return;
    await this.tentar(async () => {
      this.correcao = await api.agir<Correcao>(this.atividade, "responder", { questao: q.id, alternativa: this.escolha });
      this.questao = q;
    });
  }

  private proxima() {
    const anterior = this.questao?.id;
    this.questao = this.dados.questoes.find((q) => q.id !== anterior) ?? this.dados.questoes[0];
    this.correcao = undefined;
    this.escolha = -1;
  }

  private nomeDo(id: number) {
    const a = this.dados.assuntos.find((x) => x.id === id);
    return a ? a.titulo : "";
  }

  private fonte(f: FonteEstudo) {
    return html`<details class="externo">
      <summary>Fonte: ${f.titulo}, página ${f.pagina}</summary>
      <pre>${f.texto}</pre>
      <a href="#/acervo/documento/${f.documento}">abrir no acervo</a>
    </details>`;
  }

  private linha(a: Assunto) {
    return html`<tr class=${a.pai ? "filho" : ""} data-assunto=${a.id}>
      <td>
        ${a.pai ? a.titulo : html`<strong>${a.titulo}</strong>`}
        ${a.aviso ? html`<div class="pequeno suave">${a.aviso}</div>` : nothing}
      </td>
      <td class="numero">${a.folha ? a.material : ""}</td>
      <td class="numero">${a.vencidos}/${a.cartoes}</td>
      <td class="numero">${a.questoes}</td>
      <td class="numero">${porcento(a.acertos, a.respostas)}</td>
      <td>
        ${a.pendente
          ? html`<span class="selo rodando"><i class="girando"></i> gerando</span>`
          : html`<button type="button" aria-label=${`Gerar material de ${a.titulo}`}
              @click=${() => this.tentar(() => api.agir(this.atividade, "gerar", { assunto: a.id }))}>Gerar</button>`}
      </td>
    </tr>`;
  }

  private assuntos() {
    return html`<div class="linha-flex entre">
        <p class="suave pequeno" style="margin:0">Material é o número de trechos do acervo ligados ao assunto.</p>
        <button type="button" @click=${() => this.tentar(() => api.agir(this.atividade, "religar"), "Assuntos ligados ao acervo.")}>
          Ligar ao acervo novamente
        </button>
      </div>
      <div class="rolavel"><table>
        <thead><tr><th>Assunto</th><th class="numero">Material</th><th class="numero">A revisar</th><th class="numero">Questões</th><th class="numero">Acertos</th><th></th></tr></thead>
        <tbody>${this.dados.assuntos.map((a) => this.linha(a))}</tbody>
      </table></div>`;
  }

  private cartoes() {
    const c = this.dados.cartoes[0];
    if (!c) {
      const p = this.dados.painel.proxima_em;
      return html`<p class="vazio">
        ${this.dados.painel.cartoes ? "Nada para revisar agora." : "Ainda não há cartões. Gere material na aba Assuntos."}
        ${p ? html`<br />Próxima revisão: ${quando(p)}.` : nothing}
      </p>`;
    }
    return html`<p class="pequeno suave">${this.dados.cartoes.length} para revisar · ${this.nomeDo(c.assunto)}${c.novo ? " · novo" : ""}</p>
      <div class="cartao flash" aria-live="polite">
        <div class="frente">${c.frente}</div>
        ${this.virado ? html`<div class="verso">${c.verso}</div>` : nothing}
      </div>
      ${this.virado
        ? html`<div class="linha-flex" role="group" aria-label="Como foi?" style="justify-content:center;margin-top:0.8rem">
              ${notas.map((n, i) => html`<button type="button" ?disabled=${this.revisando} @click=${() => this.revisar(i + 1)}>${n} <kbd>${i + 1}</kbd></button>`)}
            </div>
            ${this.fonte(c.fonte)}`
        : html`<div class="linha-flex" style="justify-content:center;margin-top:0.8rem">
            <button type="button" class="primario" @click=${() => (this.virado = true)}>Mostrar resposta <kbd>espaço</kbd></button>
          </div>`}`;
  }

  private questionario() {
    const q = this.questao ?? this.dados.questoes[0];
    if (!q) return html`<p class="vazio">Ainda não há questões. Gere material na aba Assuntos.</p>`;
    const c = this.correcao;
    return html`<fieldset class="cartao alternativas">
        <p class="pequeno suave">${this.nomeDo(q.assunto)} <span class="selo ${q.situacao}">${q.situacao}</span></p>
        <p>${q.enunciado}</p>
        ${q.alternativas.map(
          (texto, i) => html`<label class=${c ? (i === c.correta ? "certa" : i === this.escolha ? "errada" : "") : ""}>
            <input type="radio" name="alternativa-${q.id}" .checked=${this.escolha === i} ?disabled=${!!c}
              @change=${() => { this.escolha = i; this.questao = q; }} />
            <span>${String.fromCharCode(65 + i)}) ${texto}</span>
          </label>`,
        )}
        ${c
          ? html`<p role="status" class=${c.acertou ? "" : "erro-texto"}>
                <strong>${c.acertou ? "Certo!" : `Errado. A correta é ${String.fromCharCode(65 + c.correta)}.`}</strong> ${c.explicacao}
              </p>
              ${this.fonte(c.fonte)}
              <p></p>
              <button type="button" class="primario" @click=${() => this.proxima()}>Próxima questão</button>`
          : html`<button type="button" class="primario" ?disabled=${this.escolha < 0} @click=${() => this.responder(q)}>Responder</button>`}
      </fieldset>`;
  }

  private painel() {
    const p = this.dados.painel;
    const fraco = this.dados.assuntos.find((a) => a.id === p.mais_fraco);
    return html`${indicadores([
      { rotulo: "Aproveitamento", valor: porcento(p.acertos, p.respostas), detalhe: `${p.respostas} respostas` },
      { rotulo: "Respostas hoje", valor: String(p.hoje) },
      { rotulo: "Cartões a revisar", valor: String(p.vencidos), detalhe: `de ${p.cartoes}` },
      { rotulo: "Questões", valor: String(p.questoes) },
    ])}
    ${fraco ? html`<p>Ponto mais fraco: <strong>${fraco.titulo}</strong> (${porcento(fraco.acertos, fraco.respostas)} de acertos). Ele aparece primeiro nas revisões.</p>` : nothing}`;
  }

  override render() {
    const d = this.dados;
    if (!d.extraido) {
      return html`<p class="suave">${this.ocupada ? html`<i class="girando"></i> Lendo o edital…` : "O edital ainda não foi lido."}</p>`;
    }
    const abas: [Aba, string][] = [
      ["assuntos", "Assuntos"],
      ["cartoes", `Cartões (${d.painel.vencidos})`],
      ["questoes", `Questões (${d.painel.questoes})`],
      ["painel", "Desempenho"],
    ];
    return html`<div class="abas" role="tablist">
        ${abas.map(
          ([id, nome]) => html`<button type="button" role="tab" aria-selected=${this.aba === id}
            @click=${() => { this.aba = id; this.virado = false; this.questao = undefined; this.correcao = undefined; this.escolha = -1; }}>${nome}</button>`,
        )}
      </div>
      <div role="tabpanel">
        ${this.aba === "assuntos" ? this.assuntos() : this.aba === "cartoes" ? this.cartoes() : this.aba === "questoes" ? this.questionario() : this.painel()}
      </div>`;
  }
}
customElements.define("pl-estudo", PlEstudo);

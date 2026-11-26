import { html, nothing } from "lit";
import { api, type Aviso } from "../api";
import { Visao, quando, tamanho } from "../base";
import type { Discos } from "../tipos";

/** Cópia de segurança para um disco escolhido, sempre com confirmação. */
export class PlCopia extends Visao<Discos> {
  static override properties = { ...Visao.properties, escolhido: { state: true }, confirmando: { state: true } };

  declare escolhido: string;
  declare confirmando: boolean;

  constructor() {
    super();
    this.escolhido = "";
    this.confirmando = false;
  }

  protected override buscar() {
    return api.obter<Discos>("/api/discos");
  }

  protected override interessa(a: Aviso) {
    return a.tipo === "copia";
  }

  private async copiar() {
    this.confirmando = false;
    await this.tentar(() => api.enviar("/api/copia", { dispositivo: this.escolhido, confirmar: true }));
    await this.recarregar();
  }

  private situacao(d: Discos) {
    const c = d.copia;
    if (c.rodando) return html`<p role="status"><i class="girando"></i> Copiando para ${c.disco}. Não retire o disco.</p>`;
    if (c.erro) return html`<div class="aviso" role="status">A cópia para ${c.disco} falhou: ${c.erro}</div>`;
    if (c.arquivo) {
      return html`<p role="status">
        Cópia concluída em ${quando(c.quando)}: <strong>${c.disco}</strong>, arquivo Palantir-copias/${c.arquivo}. O disco já pode ser retirado
        se não estava em uso antes.
      </p>`;
    }
    return nothing;
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Procurando discos…"}</p>`;
    const disco = d.discos.find((x) => x.dispositivo === this.escolhido);
    return html`<section class="cartao" aria-label="Cópia para um disco">
      <h3>Copiar para um disco ou pendrive</h3>
      <p class="pequeno suave">Documentos, atividades, configurações e históricos. Chaves e senhas não entram.</p>
      ${this.situacao(d)}
      ${d.discos.length === 0
        ? html`<p class="vazio">Nenhum disco disponível. Conecte um pendrive ou um disco externo.</p>`
        : html`<fieldset class="discos" ?disabled=${d.copia.rodando}>
            <legend class="escondido">Destino</legend>
            ${d.discos.map(
              (x) => html`<label class="linha">
                <input type="radio" name="disco" value=${x.dispositivo} .checked=${x.dispositivo === this.escolhido}
                  @change=${() => { this.escolhido = x.dispositivo; this.confirmando = false; }} />
                <span><strong>${x.nome}</strong>
                  <span class="suave pequeno">${tamanho(x.tamanho)} · ${x.sistema} · ${x.removivel ? "externo" : "interno"} · ${x.dispositivo}</span></span>
              </label>`,
            )}
          </fieldset>`}
      ${this.confirmando && disco
        ? html`<div class="aviso" role="alertdialog" aria-label="Confirmar a cópia">
            <p>Gravar uma cópia completa em <strong>${disco.nome}</strong> (${disco.dispositivo})? Nada que já está no disco é apagado.</p>
            <button type="button" class="primario" @click=${() => this.copiar()}>Confirmar a cópia</button>
            <button type="button" @click=${() => (this.confirmando = false)}>Cancelar</button>
          </div>`
        : nothing}
      <div class="linha-flex">
        <button type="button" ?disabled=${!disco || d.copia.rodando || this.confirmando} @click=${() => (this.confirmando = true)}>Copiar</button>
        <button type="button" class="discreto" @click=${() => this.recarregar()}>Procurar discos de novo</button>
      </div>
    </section>`;
  }
}
customElements.define("pl-copia", PlCopia);

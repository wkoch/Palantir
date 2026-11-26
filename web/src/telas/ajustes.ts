import { html, nothing } from "lit";
import { api, type Aviso } from "../api";
import { Visao } from "../base";
import type { Ajustes } from "../tipos";

/** Chaves, e-mail, orçamento e hosts liberados. */
export class PlAjustes extends Visao<Ajustes> {
  protected override buscar() {
    return api.obter<Ajustes>("/api/ajustes");
  }

  protected override interessa(a: Aviso) {
    return a.tipo === "ajustes";
  }

  private async segredo(e: SubmitEvent, nome: string) {
    e.preventDefault();
    const formulario = e.target as HTMLFormElement;
    const valor = String(new FormData(formulario).get("valor") ?? "").trim();
    if (!valor) return;
    if (await this.tentar(() => api.gravar(`/api/segredos/${nome}`, { valor }), "Guardado.")) formulario.reset();
  }

  private campoSecreto(nome: "openrouter" | "imap", rotulo: string, definido: boolean) {
    return html`<form @submit=${(e: SubmitEvent) => this.segredo(e, nome)}>
      <label for="segredo-${nome}">${rotulo} <span class="selo ${definido ? "ok" : "sem_chave"}">${definido ? "definida" : "não definida"}</span></label>
      <div class="linha-flex">
        <input id="segredo-${nome}" class="cresce" name="valor" type="password" autocomplete="off" required style="width:auto" />
        <button type="submit">Guardar</button>
        ${definido
          ? html`<button type="button" class="perigo" @click=${() => this.tentar(() => api.apagar(`/api/segredos/${nome}`), "Removida.")}>Remover</button>`
          : nothing}
      </div>
    </form>`;
  }

  private async email(e: SubmitEvent) {
    e.preventDefault();
    const f = new FormData(e.target as HTMLFormElement);
    const texto = (n: string) => String(f.get(n) ?? "").trim();
    const imap = {
      servidor: texto("servidor"),
      porta: Number(texto("porta")),
      usuario: texto("usuario"),
      caixa: texto("caixa") || "INBOX",
      limite: Number(texto("limite")),
      etiquetar: f.has("etiquetar"),
      seguranca: texto("seguranca"),
    };
    await this.tentar(() => api.gravar("/api/ajustes", { imap }), "E-mail configurado.");
  }

  private async orcamento(e: SubmitEvent) {
    e.preventDefault();
    const f = new FormData(e.target as HTMLFormElement);
    const orcamento = { mensal_usd: Number(f.get("mensal")), por_atividade_usd: Number(f.get("atividade")) };
    await this.tentar(() => api.gravar("/api/ajustes", { orcamento }), "Orçamento salvo.");
  }

  private async hosts(e: SubmitEvent) {
    e.preventDefault();
    const web_hosts = String(new FormData(e.target as HTMLFormElement).get("hosts") ?? "")
      .split(/[\s,]+/)
      .filter(Boolean);
    await this.tentar(() => api.gravar("/api/ajustes", { web_hosts }), "Hosts salvos.");
  }

  override render() {
    const d = this.dados;
    if (!d) return html`<p class=${this.falha ? "erro-texto" : "suave"}>${this.falha || "Carregando…"}</p>`;
    const i = d.imap;
    return html`
      <h1>Ajustes</h1>
      <h2>Modelos</h2>
      <div class="cartao">
        ${this.campoSecreto("openrouter", "Chave do OpenRouter", d.segredos.openrouter)}
        <p class="pequeno suave">
          A chave fica nesta máquina e nunca é mostrada nem enviada ao modelo.<br />
          Barato: ${d.modelos.barato} · Principal: ${d.modelos.principal} · Forte: ${d.modelos.forte} ·
          Busca semântica: ${d.modelos.embeddings || "desligada"}.
        </p>
      </div>
      <h2>E-mail (IMAP)</h2>
      <div class="cartao">
        <form @submit=${this.email} aria-label="Servidor de e-mail">
          <div class="grade">
            <div><label for="servidor">Servidor</label><input id="servidor" name="servidor" value=${i.servidor} placeholder="imap.exemplo.com" /></div>
            <div><label for="porta">Porta</label><input id="porta" name="porta" type="number" min="1" max="65535" value=${i.porta} /></div>
            <div><label for="usuario">Usuário</label><input id="usuario" name="usuario" value=${i.usuario} autocomplete="off" /></div>
            <div><label for="caixa">Caixa</label><input id="caixa" name="caixa" value=${i.caixa} /></div>
            <div><label for="limite">Mensagens por busca</label><input id="limite" name="limite" type="number" min="1" max="500" value=${i.limite} /></div>
            <div>
              <label for="seguranca">Segurança</label>
              <select id="seguranca" name="seguranca">
                ${["tls", "starttls", ...(i.seguranca === "nenhum" ? ["nenhum"] : [])].map(
                  (s) => html`<option value=${s} ?selected=${i.seguranca === s}>${s}</option>`,
                )}
              </select>
            </div>
          </div>
          <p></p>
          <div class="linha-flex entre">
            <label class="linha"><input type="checkbox" name="etiquetar" ?checked=${i.etiquetar} /> Gravar a classe como etiqueta no servidor</label>
            <button type="submit">Salvar e-mail</button>
          </div>
        </form>
        <p></p>
        ${this.campoSecreto("imap", "Senha do e-mail (use uma senha de aplicativo)", d.segredos.imap)}
      </div>
      <h2>Orçamento</h2>
      <form class="cartao" @submit=${this.orcamento} aria-label="Orçamento">
        <div class="grade">
          <div><label for="mensal">Teto mensal (US$)</label><input id="mensal" name="mensal" type="number" min="0" step="0.01" value=${d.orcamento.mensal_usd} /></div>
          <div><label for="atividade">Limite padrão por atividade (US$)</label><input id="atividade" name="atividade" type="number" min="0" step="0.01" value=${d.orcamento.por_atividade_usd} /></div>
        </div>
        <p></p>
        <button type="submit">Salvar orçamento</button>
      </form>
      <h2>Pesquisa na web</h2>
      <form class="cartao" @submit=${this.hosts} aria-label="Hosts liberados">
        <label for="hosts">Hosts que podem ser lidos sem confirmação (um por linha)</label>
        <textarea id="hosts" name="hosts" rows="5" .value=${d.web_hosts.join("\n")}></textarea>
        <button type="submit">Salvar hosts</button>
      </form>
      <h2>Arquivos</h2>
      <p class="pequeno suave">
        Configuração: ${d.caminhos.configuracao}<br />Dados: ${d.caminhos.dados}<br />Pasta de entrada: ${d.caminhos.entrada}
      </p>
    `;
  }
}
customElements.define("pl-ajustes", PlAjustes);

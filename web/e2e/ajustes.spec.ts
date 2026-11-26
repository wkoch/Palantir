import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { abrir, criar, estado, expect, test } from "./base";

test("chave, e-mail, orçamento e hosts são gravados sem expor segredos", async ({ page, sistema }) => {
  await abrir(page, sistema, "#/ajustes");
  const chave = page.locator("#segredo-openrouter");
  await expect(chave).toHaveValue("");
  await expect(page.locator('label[for="segredo-openrouter"]')).toContainText("definida");

  await chave.fill("recusada");
  await chave.press("Enter");
  await expect(chave).toHaveValue("");
  await page.getByRole("link", { name: "Fila" }).click();
  await criar(page, "Conversa", "teste de chave");
  await expect(estado(page)).toHaveText("esperando você");
  await expect(page.getByRole("status")).toContainText("recusou");
  await expect(page.locator('[data-teste="modelos"]')).toHaveText("chave recusada");

  await page.getByRole("link", { name: "Ajustes" }).click();
  await page.locator("form", { has: chave }).getByRole("button", { name: "Remover" }).click();
  await page.getByRole("link", { name: /Fila/ }).click();
  await expect(page.getByText("Sem chave do OpenRouter")).toBeVisible();
  await page.getByRole("link", { name: "Configurar a chave" }).click();
  await chave.fill("chave-nova-secreta-123");
  await chave.press("Enter");
  await expect(page.locator('[data-teste="modelos"]')).toHaveText("modelos ok");
  expect(await page.content()).not.toContain("chave-nova-secreta-123");

  const email = page.getByRole("form", { name: "Servidor de e-mail" });
  await email.getByLabel("Mensagens por busca").fill("7");
  await email.getByLabel("Caixa").fill("");
  await email.getByLabel("Gravar a classe").uncheck();
  await email.getByRole("button", { name: "Salvar e-mail" }).click();
  await expect(page.getByRole("log")).toContainText("E-mail configurado");
  await email.getByLabel("Segurança").selectOption("tls");
  await email.getByLabel("Servidor", { exact: true }).fill("não é um host");
  await email.getByRole("button", { name: "Salvar e-mail" }).click();
  await expect(page.getByRole("log")).toContainText("não é um nome de host");

  const orcamento = page.getByRole("form", { name: "Orçamento" });
  await orcamento.getByLabel("Teto mensal").fill("33");
  await orcamento.getByRole("button", { name: "Salvar orçamento" }).click();
  await expect(page.getByRole("log")).toContainText("Orçamento salvo");

  const hosts = page.getByRole("form", { name: "Hosts liberados" });
  await hosts.getByRole("textbox").fill("127.0.0.1\nexemplo.org, docs.exemplo.org");
  await hosts.getByRole("button", { name: "Salvar hosts" }).click();
  await expect(page.getByRole("log")).toContainText("Hosts salvos");
  await hosts.getByRole("textbox").fill("http://exemplo.org/caminho");
  await hosts.getByRole("button", { name: "Salvar hosts" }).click();
  await expect(page.getByRole("log")).toContainText("não é um nome de host");

  await page.reload();
  await expect(page.getByLabel("Mensagens por busca")).toHaveValue("7");
  await expect(page.getByLabel("Caixa")).toHaveValue("INBOX");
  await expect(page.getByLabel("Teto mensal")).toHaveValue("33");
  await expect(page.getByRole("form", { name: "Hosts liberados" }).getByRole("textbox")).toHaveValue(
    "127.0.0.1\nexemplo.org\ndocs.exemplo.org",
  );
  const diario = await readFile(join(sistema.pasta, "dados", "diario.jsonl"), "utf8");
  expect(diario).not.toContain("chave-nova-secreta-123");
  const config = await readFile(join(sistema.pasta, "config", "ajustes.toml"), "utf8");
  expect(config).not.toContain("secreta");

  await page.getByRole("link", { name: "Painel" }).click();
  await expect(page.getByRole("progressbar", { name: "Uso do teto mensal" })).toBeVisible();
  await expect(page.getByRole("cell", { name: "segredo openrouter" }).first()).toBeVisible();
  await expect(page.getByRole("cell", { name: "ajustes" }).first()).toBeVisible();
});

test("layout em tela estreita não rola na horizontal", async ({ page, sistema }) => {
  await sistema.importarExemplos();
  await page.setViewportSize({ width: 390, height: 800 });
  for (const rota of ["", "#/acervo", "#/painel", "#/ajustes"]) {
    await abrir(page, sistema, rota);
    await expect(page.getByRole("main").getByRole("heading").first()).toBeAttached();
    const sobra = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(sobra, `rota ${rota}`).toBeLessThanOrEqual(1);
  }
});

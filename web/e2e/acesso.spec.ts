import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { abrir, criar, estado, expect, test } from "./base";

test("sem a chave, a interface não mostra nada e explica como entrar", async ({ page, sistema }) => {
  await page.goto(sistema.url);
  await expect(page.getByText("Esta janela não tem acesso aos seus dados.")).toBeVisible();
  await expect(page.getByText("palantird abrir")).toBeVisible();
  await expect(page.getByRole("navigation")).toBeHidden();

  // O endereço que o atalho abre dá acesso, e a chave sai da barra de endereço.
  await page.goto(`${sistema.url}/#entrar=${sistema.chave}`);
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
  expect(page.url()).toBe(`${sistema.url}/#/`);
  await page.reload();
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();

  // Um endereço com uma chave inventada, como o que outro site poderia abrir,
  // não troca a boa.
  const recarregada = page.waitForEvent("load");
  await page.goto(`${sistema.url}/#entrar=errada`);
  await recarregada;
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem("palantir.chave"))).toBe(sistema.chave);
});

test("o atalho do sistema abre a interface a partir de um arquivo local", async ({ page, sistema }) => {
  const arquivo = join(sistema.pasta, "entrar.html");
  await writeFile(arquivo, `<meta http-equiv="refresh" content="0;url=${sistema.url}/#entrar=${sistema.chave}">`);
  await page.goto(`file://${arquivo}`);
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
  expect(page.url()).toBe(`${sistema.url}/#/`);
});

test("uma aba sem acesso se recupera quando outra recebe a chave", async ({ context, page, sistema }) => {
  await page.goto(sistema.url);
  await expect(page.getByText("Esta janela não tem acesso aos seus dados.")).toBeVisible();
  const outra = await context.newPage();
  await outra.goto(`${sistema.url}/#entrar=${sistema.chave}`);
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
});

test("um endereço sozinho abre o site; um pedido vai para a IA", async ({ context, page, sistema }) => {
  // Nada sai da máquina: os sites do teste são respondidos aqui.
  await context.route(/^https:\/\/(www\.)?exemplo\.com\//, (r) =>
    r.fulfill({ contentType: "text/html", body: "<title>Site de exemplo</title><h1>Site de exemplo</h1>" }),
  );
  await abrir(page, sistema);
  const campo = page.locator("textarea[name=objetivo]");
  await campo.fill("exemplo.com/pagina?x=1");
  await expect(page.getByRole("button", { name: "Abrir o site" })).toBeVisible();
  const [aba] = await Promise.all([context.waitForEvent("page"), campo.press("Enter")]);
  await expect(aba.getByRole("heading", { name: "Site de exemplo" })).toBeVisible();
  expect(aba.url()).toBe("https://exemplo.com/pagina?x=1");
  expect(await aba.evaluate(() => window.opener === null && document.referrer === "")).toBeTruthy();
  await aba.close();
  // O Palantír continua onde estava, sem criar atividade.
  await expect(campo).toHaveValue("");
  await expect(page.getByText("Nenhuma atividade ainda")).toBeVisible();

  // Um nome de arquivo ou uma frase com endereço são pedidos.
  for (const pedido of ["contrato.pdf", "resuma https://exemplo.com/pagina", "javascript:alert(1)"]) {
    await campo.fill(pedido);
    await expect(page.getByRole("button", { name: "Começar" })).toBeVisible();
  }
  await criar(page, "Conversa", "O que há no acervo?");
  await expect(estado(page)).toHaveText("pronta");

  // Na conversa também.
  const mensagem = page.getByRole("textbox", { name: "Mensagem" });
  await mensagem.fill("https://www.exemplo.com/");
  const [segunda] = await Promise.all([context.waitForEvent("page"), mensagem.press("Enter")]);
  await expect(segunda.getByRole("heading", { name: "Site de exemplo" })).toBeVisible();
  await segunda.close();
  await expect(page.locator('[data-origem="usuario"]')).toHaveCount(1);
});

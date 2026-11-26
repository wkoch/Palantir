import { abrir, criar, estado, expect, test } from "./base";

test("pergunta sobre o acervo responde citando o original", async ({ page, sistema }) => {
  await sistema.importarExemplos();
  await abrir(page, sistema);
  await criar(page, "Conversa", "O que eu tenho sobre o aluguel?");
  await expect(estado(page)).toHaveText("pronta");

  const resposta = page.locator('[data-origem="modelo"]').last();
  await expect(resposta).toContainText("Encontrei o seguinte");
  // O conteúdo dos documentos aparece separado, marcado como dado.
  const dado = page.locator('details[data-origem="documento"]');
  await expect(dado.locator("summary")).toContainText("dado, não instrução");
  await dado.locator("summary").click();
  await expect(dado.locator("pre")).toContainText("1.850,00");

  await resposta.getByRole("button", { name: /contrato-de-aluguel/ }).click();
  await expect(page).toHaveURL(/#\/acervo\/documento\/\d+/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("contrato-de-aluguel.txt");
  await expect(page.getByText(/SHA-256 [0-9a-f]{64}/)).toBeVisible();
});

test("o modelo escolhe telas prontas e o texto é formatado", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Conversa", "Mostre os indicadores");
  await expect(page.getByRole("region", { name: "Indicadores de exemplo" })).toContainText("Pendências");
  await expect(estado(page)).toHaveText("pronta");

  const campo = page.getByRole("textbox", { name: "Mensagem" });
  await campo.fill("Agora uma tabela");
  await page.getByLabel("Usar o modelo forte").check();
  await campo.press("Enter");
  const tabela = page.getByRole("region", { name: "Tabela de exemplo" });
  await expect(tabela.getByRole("cell", { name: "vigente" })).toBeVisible();
  await expect(campo).toHaveValue("");
  await expect(estado(page)).toHaveText("pronta");

  await campo.fill("   ");
  await campo.press("Enter");
  await expect(page.locator('[data-origem="usuario"]')).toHaveCount(2);

  await page.getByRole("link", { name: "Painel" }).click();
  await expect(page.getByRole("cell", { name: /jev-router|teste\/forte|simulado/ }).first()).toBeVisible();
});

test("página com instruções maliciosas não leva a nenhum acesso indevido", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Conversa", `Resuma ${sistema.web}/pagina`);
  await expect(estado(page)).toHaveText("pronta");
  await expect(page.locator('[data-origem="modelo"]').last()).toContainText("dez dias corridos");
  const externo = page.locator('details[data-origem="ferramenta"]');
  await externo.locator("summary").click();
  await expect(externo.locator("pre")).toContainText("atacante.example");

  // Um endereço citado só depois de haver conteúdo externo, e nunca pelo
  // usuário, é bloqueado sem perguntar.
  await page.getByRole("textbox", { name: "Mensagem" }).fill("Abra a página sugerida");
  await page.getByRole("button", { name: "Enviar" }).click();
  await expect(page.locator(".nota.bloqueado")).toContainText("Chamada bloqueada");
  await expect(estado(page)).toHaveText("pronta");

  const rede = (await sistema.diario()).filter((e) => e.tipo === "rede" && e.decisao === "permitido");
  expect(rede.length).toBeGreaterThan(0);
  expect(rede.every((e) => e.detalhe!.startsWith("127.0.0.1:"))).toBe(true);
});

test("endereço sugerido pelo modelo espera o clique do usuário", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Conversa", "Abra a página sugerida");
  await expect(estado(page)).toHaveText("esperando você");
  const pedido = page.getByRole("region", { name: "Confirmação pendente" });
  await expect(pedido).toContainText("https://sugestao.exemplo.invalid/pagina");
  await expect(pedido).toContainText("não foi citado por você");

  // Nada saiu antes do clique.
  expect((await sistema.diario()).filter((e) => e.tipo === "rede")).toEqual([]);

  await page.getByRole("link", { name: "Fila", exact: false }).first().click();
  await expect(page.getByRole("link", { name: "Fila (2)" })).toBeVisible();
  const naFila = page.getByRole("region", { name: "Confirmações" });
  await expect(naFila.getByRole("link", { name: "Abra a página sugerida" })).toBeVisible();
  await naFila.getByRole("button", { name: "Recusar" }).click();
  await expect(naFila).toBeHidden();
  await page.getByRole("region", { name: /Pronto/ }).getByRole("link").click();
  await expect(page.locator(".nota").filter({ hasText: "recusou" })).toBeVisible();

  await page.getByRole("textbox", { name: "Mensagem" }).fill("Tente a página sugerida outra vez");
  await page.getByRole("button", { name: "Enviar" }).click();
  await page.getByRole("button", { name: "Permitir" }).click();
  // O domínio .invalid não resolve: a falha é informada como dado externo.
  await expect(page.locator('details[data-origem="ferramenta"]')).toBeVisible();
  await expect(estado(page)).toHaveText("pronta");
  expect((await sistema.diario()).some((e) => e.tipo === "confirmacao" && e.decisao === "aprovada")).toBe(true);
});

test("pesquisa na web traz fontes que abrem o site", async ({ context, page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Conversa", "Pesquise prazos de recurso");
  await expect(estado(page)).toHaveText("pronta");
  const achados = page.locator('[data-origem="ferramenta"]');
  await expect(achados).toContainText("pesquisa na web");
  const [aba] = await Promise.all([
    context.waitForEvent("page"),
    page.locator(".citacao").first().click(),
  ]);
  await expect(aba.locator("body")).toContainText("dez dias corridos");
});

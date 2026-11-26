import { abrir, criar, estado, expect, test } from "./base";

test("a fila de atenção é a tela inicial e uma conversa aparece nela", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await expect(page.getByText("Nenhuma atividade ainda")).toBeVisible();
  await expect(page.locator('[data-teste="modelos"]')).toHaveText("modelos ok");

  const campo = page.locator("textarea[name=objetivo]");
  await campo.fill("Bom dia");
  await campo.press("Shift+Enter");
  await expect(campo).toHaveValue("Bom dia\n");
  await campo.press("Enter");
  await expect(page).toHaveURL(/#\/atividade\/1/);
  await expect(page.getByText("modo de simulação")).toBeVisible();
  await expect(estado(page)).toHaveText("pronta");

  await page.getByRole("link", { name: "← Fila de atenção" }).click();
  const pronto = page.getByRole("region", { name: "Pronto" });
  await expect(pronto.getByRole("link", { name: /Bom dia/ })).toContainText("Conversa");
  await expect(pronto.getByTitle("Custo desta atividade")).toContainText("US$");
  await expect(page.getByText(/Gasto no mês/)).toBeVisible();
});

test("pausar, retomar, limitar e arquivar uma atividade", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Conversa", "Olá, tudo bem");
  await expect(estado(page)).toHaveText("pronta");

  await page.getByText("Limite e histórico").click();
  await page.getByLabel("Limite desta atividade").fill("0");
  await page.getByRole("button", { name: "Salvar limite" }).click();
  await expect(page.getByRole("log")).toContainText("Limite atualizado");

  // Sem orçamento, a próxima chamada pausa a atividade.
  await page.getByRole("textbox", { name: "Mensagem" }).fill("Mais uma pergunta");
  await page.getByRole("button", { name: "Enviar" }).click();
  await expect(estado(page)).toHaveText("pausada");
  await expect(page.getByRole("status")).toContainText("orçamento esgotado");

  await page.getByLabel("Limite desta atividade").fill("2");
  await page.getByRole("button", { name: "Salvar limite" }).click();
  await page.getByRole("button", { name: "Retomar" }).click();
  await expect(estado(page)).toHaveText("pronta");
  await expect(page.locator('[data-origem="modelo"]')).toHaveCount(2);

  await page.getByRole("button", { name: "Arquivar" }).click();
  await expect(estado(page)).toHaveText("arquivada");
  await page.getByRole("link", { name: "← Fila de atenção" }).click();
  await expect(page.getByText("Nenhuma atividade ainda")).toBeVisible();
  await page.getByLabel("Mostrar arquivadas").check();
  await expect(page.getByRole("region", { name: "Arquivadas" }).getByRole("link")).toHaveCount(1);
});

test("atalhos de teclado navegam entre as telas", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await page.locator("body").press("?");
  const ajuda = page.getByRole("dialog", { name: "Atalhos de teclado" });
  await expect(ajuda).toContainText("Ctrl+Alt+F2");
  await ajuda.getByRole("button", { name: "Fechar" }).click();
  await expect(ajuda).toBeHidden();
  await page.getByRole("button", { name: "Atalhos de teclado" }).click();
  await page.keyboard.press("Escape");

  for (const [tecla, titulo] of [["a", "Acervo"], ["p", "Painel"], ["j", "Ajustes"]] as const) {
    await page.locator("body").press("g");
    await page.locator("body").press(tecla);
    await expect(page.getByRole("heading", { level: 1, name: titulo })).toBeVisible();
  }
  await page.locator("body").press("g");
  await page.locator("body").press("f");
  await expect(page.getByRole("form", { name: "Nova atividade" })).toBeVisible();
  await page.locator("body").press("g");
  await page.locator("body").press("a");
  await expect(page.getByRole("heading", { level: 1, name: "Acervo" })).toBeVisible();
  await page.locator("body").press("n");
  await expect(page.locator("textarea[name=objetivo]")).toBeFocused();
  // Dentro de um campo, as letras são texto.
  await page.keyboard.type("gato");
  await expect(page.locator("textarea[name=objetivo]")).toHaveValue("gato");
});

test("reiniciar o daemon preserva as atividades e a interface reconecta", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Conversa", "Guarde esta conversa");
  await expect(estado(page)).toHaveText("pronta");

  await sistema.desligar();
  await expect(page.getByRole("alert")).toHaveText("daemon desconectado");
  await page.getByRole("textbox", { name: "Mensagem" }).fill("não vai chegar");
  await page.getByRole("button", { name: "Enviar" }).click();
  await expect(page.getByRole("log")).toContainText("não está respondendo");
  // O texto digitado não se perde.
  await expect(page.getByRole("textbox", { name: "Mensagem" })).toHaveValue("não vai chegar");

  await sistema.ligar();
  await expect(page.locator('[data-teste="modelos"]')).toBeVisible();
  await page.reload();
  await expect(page.locator('[data-origem="usuario"]')).toHaveText("Guarde esta conversa");
  await expect(estado(page)).toHaveText("pronta");
});

test("atividade inexistente mostra o erro", async ({ page, sistema }) => {
  await abrir(page, sistema, "#/atividade/999");
  await expect(page.getByText("atividade não encontrada")).toBeVisible();
  await page.goto(`${sistema.url}/#/acervo/documento/999`);
  await expect(page.getByText("documento não encontrado")).toBeVisible();
});

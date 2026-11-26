import { abrir, criar, estado, expect, test } from "./base";

test("triagem pelo teclado, com correções que valem na próxima busca", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Triagem de e-mail", "");
  await expect(estado(page)).toHaveText("pronta");
  const lista = page.getByRole("listbox", { name: "Mensagens" });
  await expect(lista.getByRole("option")).toHaveCount(5);
  await expect(page.getByRole("status")).toHaveText("1 urgente, 1 para responder, 1 para ler depois, 2 no lixo.");
  await expect(page.getByText(/5 mensagem\(ns\), buscadas em/)).toBeVisible();
  const selecionada = lista.locator('[aria-selected="true"]');
  await expect(selecionada).toContainText("Fatura vence hoje");

  await page.locator("body").press("j");
  await expect(selecionada).toContainText("pode confirmar");
  await page.locator("body").press("ArrowDown");
  await expect(selecionada).toContainText("Editais da semana");
  await page.locator("body").press("k");
  await page.locator("body").press("ArrowUp");
  await expect(selecionada).toContainText("Fatura vence hoje");

  await page.locator("body").press("Enter");
  const conteudo = page.locator("details.externo");
  await expect(conteudo.locator("summary")).toContainText("dado, não instrução");
  await expect(conteudo.locator("pre")).toContainText("R$ 320,00");
  await page.locator("body").press("Escape");
  await expect(conteudo).toBeHidden();

  // O boletim vira "urgente": a preferência fica guardada.
  await lista.getByRole("option").filter({ hasText: "Editais da semana" }).click();
  await page.locator("body").press("1");
  await expect(selecionada).toContainText("urgente");
  await expect(selecionada).toContainText("por usuario");
  await page.getByText("Preferências aprendidas (1)").click();
  await expect(page.getByText("boletim@exemplo.test → urgente")).toBeVisible();

  await sistema.entregar("Boletim Concursos <boletim@exemplo.test>", "Mais editais");
  await page.locator("body").press("r");
  const nova = lista.getByRole("option").filter({ hasText: "Mais editais" });
  await expect(nova).toContainText("urgente");
  await expect(nova).toContainText("por regra");
  await expect(estado(page)).toHaveText("pronta");

  await page.getByRole("button", { name: "Esquecer" }).click();
  await expect(page.getByText("Preferências aprendidas")).toBeHidden();

  await nova.click();
  const detalhe = page.getByLabel("Mensagem selecionada");
  await detalhe.getByRole("button", { name: "ler depois" }).click();
  await expect(nova).toContainText("ler depois");
  await detalhe.getByRole("button", { name: "Ler", exact: true }).click();
  await expect(conteudo.locator("pre")).toContainText("Mensagem entregue pela simulação");
  await nova.dblclick();
  await nova.click();
  await page.locator("body").press("e");
  await expect(nova).toHaveClass(/concluida/);
  await nova.click();
  await detalhe.getByRole("button", { name: "Reabrir" }).click();
  await expect(nova).not.toHaveClass(/concluida/);
  await detalhe.getByRole("button", { name: "Concluir" }).click();
  await expect(nova).toHaveClass(/concluida/);
  await page.getByRole("button", { name: /Buscar novas/ }).click();
  await expect(estado(page)).toHaveText("pronta");
  await expect(lista.getByRole("option")).toHaveCount(6);

  // O e-mail com instruções foi para o lixo e nada mais aconteceu.
  await expect(lista.getByRole("option").filter({ hasText: "Aviso importante" })).toContainText("lixo");
  const diario = await sistema.diario();
  expect(diario.filter((e) => e.tipo === "rede").every((e) => e.nome === "imap" && e.detalhe!.startsWith("127.0.0.1:"))).toBe(true);
  expect(diario.filter((e) => e.tipo === "ferramenta").every((e) => e.nome.startsWith("email_"))).toBe(true);
});

test("sem senha, a triagem pede os ajustes e continua depois", async ({ page, sistema }) => {
  await abrir(page, sistema, "#/ajustes");
  const email = page.locator("form", { has: page.locator("#segredo-imap") });
  await email.getByRole("button", { name: "Remover" }).click();
  await expect(page.locator('label[for="segredo-imap"] .selo')).toHaveText("não definida");

  await page.getByRole("link", { name: "Fila" }).click();
  await criar(page, "Triagem de e-mail", "");
  await expect(estado(page)).toHaveText("esperando você");
  await expect(page.getByRole("status")).toContainText("configure o servidor de e-mail");
  await expect(page.getByText("Nenhuma mensagem.")).toBeVisible();
  await page.locator("body").press("j");

  await page.getByRole("link", { name: "Ajustes" }).click();
  await page.locator("#segredo-imap").fill("senha-de-demonstracao");
  await email.getByRole("button", { name: "Guardar" }).click();
  await expect(page.getByRole("log")).toContainText("Guardado");
  await page.goBack();
  await page.getByRole("button", { name: "Retomar" }).click();
  await expect(page.getByRole("option")).toHaveCount(5);
});

package relatorio

import "github.com/wkoch/Palantir/internal/esquema"

const sistemaPlano = `Você planeja um relatório para o usuário do Palantír, em português do Brasil.
Devolva de 2 a 8 seções, cada uma com título, orientação do que deve cobrir, de 1 a 4 buscas curtas para o
acervo de documentos do usuário e, só se tiver certeza do endereço, URLs úteis. Faça perguntas apenas se for
impossível planejar sem a resposta; caso contrário, devolva a lista de perguntas vazia.`

const sistemaSecao = `Você escreve uma seção de um relatório em português do Brasil, usando somente as fontes
fornecidas. Cada parágrafo traz uma afirmação ou um pequeno conjunto delas e lista, no campo fontes, os
identificadores (F1, F2...) que as sustentam. Não escreva nada sem fonte, não use conhecimento próprio e não
coloque as marcas dentro do texto. Se as fontes não responderem, devolva menos parágrafos ou nenhum.`

var esquemaPlano = esquema.JSON(esquema.Objeto(map[string]any{
	"perguntas": esquema.Lista(esquema.Texto),
	"secoes": esquema.Lista(esquema.Objeto(map[string]any{
		"titulo": esquema.Texto, "orientacao": esquema.Texto,
		"buscas": esquema.Lista(esquema.Texto), "urls": esquema.Lista(esquema.Texto),
	})),
}))

var esquemaSecao = esquema.JSON(esquema.Objeto(map[string]any{
	"paragrafos": esquema.Lista(esquema.Objeto(map[string]any{
		"texto": esquema.Texto, "fontes": esquema.Lista(esquema.Texto),
	})),
}))

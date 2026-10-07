# further-api

`further-api` est une passerelle auto-hébergée pour les modèles d'IA. Elle réunit le routage des fournisseurs, les comptes, les clés API, la facturation et une console d'administration.

[简体中文](./README.md) · [English](./README.en.md) · [繁體中文](./README.zh_TW.md) · [日本語](./README.ja.md)

## Démarrage rapide

```powershell
git clone https://github.com/dqt081228OHVGFY/further-api.git
Set-Location further-api
Copy-Item compose.env.example .env
```

Renseignez `POSTGRES_PASSWORD`, `REDIS_PASSWORD` et `SESSION_SECRET`, `CRYPTO_SECRET` dans `.env`, puis lancez `docker compose up -d --build`. Ouvrez <http://localhost:3000> pour terminer l'installation. En production, configurez HTTPS et l'origine exacte des cookies de session.

Pour développer localement, utilisez Go 1.27.1 ou une version compatible plus récente, Bun 1.4.2 et Node.js. Dans `web/`, exécutez `bun install --frozen-lockfile` puis `bun run build`; à la racine, lancez `go run .`.

## Sources et état de la migration

Les sources de l'interface se trouvent dans `further-ui/` (pages, scripts et styles) et `web/src/further/` (pages publiques, authentification, espaces fonctionnels indépendants et accès aux API). La commande complète `bun run build` compile les points d'entrée, puis exécute `web/scripts/build-further.mjs` pour publier l'accueil, la connexion, la console utilisateur, l'administration et les cartes prépayées dans `web/dist/`. L'ancien arbre de routes des pages n'est plus compilé. Exécuter uniquement `rsbuild build` ne produit pas une version complète.

Le serveur exécute directement le service Go further-api. Les principaux traitements de l'authentification et des sessions, de la liaison des identités, des contrôles de sécurité, des transactions de configuration, des abonnements, du routage et de la conversion des protocoles, des connexions HTTP/SSE/WebSocket aux fournisseurs, de la facturation, des tâches asynchrones ainsi que des commandes et notifications de paiement ont été réécrits et intégrés aux points d'entrée de production. Les DTO des protocoles officiels, les SDK tiers, les constantes des modèles et des régions, les outils de conversion de données et les migrations des données existantes sont conservés. Les contributions historiques et leurs obligations de licence restent applicables. Consultez [l'implémentation et la migration du backend](./docs/backend-architecture.md) pour le périmètre, les vérifications et les modalités de mise à niveau.

Les droits d'auteur historiques, la licence et les obligations d'attribution figurent dans [LICENSE](./LICENSE), [NOTICE](./NOTICE) et les [licences tierces](./THIRD-PARTY-LICENSES.md). Le nom du produit est **further-api**.

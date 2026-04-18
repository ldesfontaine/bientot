# Bientôt

*Monitoring agent + dashboard modulaire en Go, sécurisé par mTLS et signature Ed25519.*

> 🚧 **Branche `refonte` — refonte complète en cours.** Voir [REFONTE.md](./REFONTE.md) pour la roadmap.

## Status

La branche `refonte` réécrit le projet depuis zéro, avec une archi propre et une sécurité niveau production dès le départ.

La branche `main` contient l'ancienne version — toujours fonctionnelle, mais plus maintenue activement.

Pour l'instant, clonez `main` si vous voulez quelque chose qui tourne.

## Architecture

Un agent léger par machine remonte des métriques et événements vers un dashboard central, tout en Go. La communication est sécurisée en trois couches : mTLS obligatoire (certificats X.509 signés par une CA interne step-ca), JWT court-vécu pour l'identité applicative, et signature Ed25519 sur chaque message. Le dashboard stocke les données dans SQLite et expose une interface HTMX/uPlot. Pour les détails, voir [REFONTE.md](./REFONTE.md).

## Documentation

- [REFONTE.md](./REFONTE.md) — roadmap, décisions d'architecture, paliers

## Quick start

*À venir une fois le palier 6 atteint (agent production-ready).*

Pour tester l'ancienne version : `git checkout main` et suivre le README de `main`.
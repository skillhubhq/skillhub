import { EvaluationService } from "../gen/api/evaluation/v1/evaluation_service_pb.js";
export * from "../gen/api/evaluation/v1/evaluation_service_pb.js";
import { GovernanceService } from "../gen/api/governance/v1/governance_service_pb.js";
export * from "../gen/api/governance/v1/governance_service_pb.js";
import { createClient, type Transport } from "@connectrpc/connect";
import { CommunityService } from "../gen/api/community/v1/community_service_pb.js";
import { AdminService } from "../gen/api/admin/v1/admin_service_pb.js";
import { AuthService } from "../gen/api/auth/v1/auth_service_pb.js";
import { CatalogService } from "../gen/api/catalog/v1/catalog_service_pb.js";
import { PublishingService } from "../gen/api/publishing/v1/publishing_service_pb.js";
import { RegistryService } from "../gen/api/registry/v1/registry_service_pb.js";

export * from "../gen/api/community/v1/community_service_pb.js";
export * from "../gen/api/admin/v1/admin_service_pb.js";
export * from "../gen/api/auth/v1/auth_service_pb.js";
export * from "../gen/api/catalog/v1/catalog_service_pb.js";
export * from "../gen/api/common/v1/types_pb.js";
export * from "../gen/api/publishing/v1/publishing_service_pb.js";
export * from "../gen/api/registry/v1/registry_service_pb.js";
export * from "../gen/common/registry/v1/taxonomy_pb.js";

// SDK 仅提供协议客户端；凭据隔离、安装和项目恢复由共享 Go 内核负责。
export function createHubClients(transport: Transport) {
  return {
    evaluation: createClient(EvaluationService, transport),
    governance: createClient(GovernanceService, transport),
    community: createClient(CommunityService, transport),
    registry: createClient(RegistryService, transport),
    auth: createClient(AuthService, transport),
    catalog: createClient(CatalogService, transport),
    publishing: createClient(PublishingService, transport),
    admin: createClient(AdminService, transport),
  };
}

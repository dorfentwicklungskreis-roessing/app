import Foundation

extension DorfApi {
    /// Sends an error report.
    ///
    /// The entrance deliberately works without a login (see
    /// `backend/internal/api/error_reports.go`): the failures worth knowing
    /// about are exactly the ones where signing in is what broke. A token is
    /// sent along whenever there is one — then the report hangs on the
    /// account and the Dorfentwicklungskreis can ask back.
    @discardableResult
    func sendErrorReport(_ input: ErrorReportInput) async throws -> ErrorReportEcho {
        try await schicke("POST", "api/v1/error-reports", rumpf: input)
    }
}

import Combine
import Foundation

/// The profile in the rental platform: telephone and address, and the way to
/// ask for permission to lend things out.
///
/// Two things are worth knowing about it. First, the platform creates the
/// account by itself on the first call with a Rössing-ID — the app does
/// nothing for that (`docs/mieten-api.md`, route 7). Second, the profile is
/// **not** a copy of the village profile: it lives over there, it is what the
/// lender sees when the device is handed over, and the platform decides when
/// it is complete enough (`profileComplete`, `missingFields`).
///
/// Nothing here works out who may lend: `lenderStatus` arrives from the
/// platform, and the lender's side appears only when it says `approved`.
final class RentalProfileModel: ObservableObject {
    @Published private(set) var profile: RentalProfile?
    @Published private(set) var loading = false
    @Published private(set) var saving = false
    @Published private(set) var trouble: RentalTrouble?
    /// What the platform said after „ich möchte verleihen" — its own
    /// sentence, shown as it stands.
    @Published private(set) var lenderMessage: String?
    @Published private(set) var askingToLend = false

    // The form. Filled from the profile once it arrives, then owned by the
    // person typing.
    @Published var name = ""
    @Published var phone = ""
    @Published var addressStreet = ""
    @Published var addressZip = ""
    @Published var addressCity = ""

    private var fetched = false
    private let source: RentalSource

    init(source: RentalSource) {
        self.source = source
    }

    var needsSignIn: Bool { trouble?.wantsSignIn == true }

    /// The note above the form.
    var hint: String? { trouble?.message }

    /// What the platform is still missing — its list, in German.
    var missingLabels: [String] { profile?.missingLabels ?? [] }

    func load() async {
        if fetched { return }
        await fetch()
    }

    func refresh() async {
        await fetch()
    }

    private func fetch() async {
        if loading { return }
        loading = true
        defer { loading = false }
        do {
            let answer = try await source.profile()
            apply(answer.asProfile())
            fetched = true
            trouble = nil
        } catch {
            trouble = RentalTrouble(error)
        }
    }

    private func apply(_ fresh: RentalProfile) {
        profile = fresh
        name = fresh.name
        phone = fresh.phone
        addressStreet = fresh.addressStreet
        addressZip = fresh.addressZip
        addressCity = fresh.addressCity
    }

    /// Sends what is in the form. Only fields that carry something go out —
    /// the platform changes exactly what it is given, and an empty value in a
    /// sent field is a `bad_request`, not a way to clear it.
    @discardableResult
    func save() async -> Bool {
        if saving { return false }
        saving = true
        defer { saving = false }
        let patch = RentalProfilePatchDto(
            name: name.rentalNonEmpty,
            phone: phone.rentalNonEmpty,
            addressStreet: addressStreet.rentalNonEmpty,
            addressZip: addressZip.rentalNonEmpty,
            addressCity: addressCity.rentalNonEmpty
        )
        do {
            let answer = try await source.updateProfile(patch)
            apply(answer.asProfile())
            trouble = nil
            return true
        } catch {
            // What was typed stays where it is; nobody fills in an address
            // twice because the network hiccuped.
            trouble = RentalTrouble(error)
            return false
        }
    }

    /// Asks to become a lender. The answer is a receipt, not a permission —
    /// somebody decides that by hand, in the web version.
    func askToLend() async {
        if askingToLend { return }
        askingToLend = true
        defer { askingToLend = false }
        do {
            let answer = try await source.requestLender()
            lenderMessage = answer.message.rentalNonEmpty
            trouble = nil
            // The status moved; the platform's own answer is what counts, so
            // it is fetched rather than patched together here.
            if let fresh = try? await source.profile() { apply(fresh.asProfile()) }
        } catch {
            trouble = RentalTrouble(error)
        }
    }

    func clearTrouble() {
        trouble = nil
    }
}

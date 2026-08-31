import SwiftUI

/// „Meine Buchungen" — what I asked for, what was confirmed, and where to
/// pick it up once it was.
struct RentalBookingsView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var model: RentalBookingsModel?

    var body: some View {
        Group {
            if let model {
                RentalBookingsList(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Meine Buchungen")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let existing = model ?? RentalBookingsModel(source: .from(umgebung.rental))
            model = existing
            await existing.load()
        }
    }
}

struct RentalBookingsList: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @ObservedObject var model: RentalBookingsModel
    @State private var signingIn = false
    /// The booking somebody is about to withdraw. Nobody cancels by accident.
    @State private var aboutToCancel: RentalBooking?

    var body: some View {
        List {
            if let hint = model.hint {
                Section {
                    if model.needsSignIn {
                        // The one case where „try again" would be a lie: the
                        // token is the problem, and a fresh sign-in is the
                        // fix. See `ANMELDE_SCOPES`.
                        RentalNotice(text: hint, actionTitle: "Neu anmelden") {
                            Task { await signIn() }
                        }
                        .accessibilityIdentifier("rental-bookings-sign-in")
                    } else {
                        RentalNotice(text: hint) {
                            Task { await model.refresh() }
                        }
                    }
                }
            }

            Section {
                if model.loading && model.bookings.isEmpty {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Buchungen werden geholt …").foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("rental-bookings-loading")
                }

                ForEach(model.bookings) { booking in
                    VStack(alignment: .leading, spacing: 8) {
                        RentalBookingRow(booking: booking)

                        // Withdrawing is offered because the platform says it
                        // is allowed — the app does not work that out.
                        if booking.canCancel {
                            Button(role: .destructive) {
                                aboutToCancel = booking
                            } label: {
                                HStack(spacing: 8) {
                                    if model.cancelling.contains(booking.id) {
                                        ProgressView().controlSize(.small)
                                    }
                                    Text("Buchung zurückziehen")
                                }
                            }
                            .buttonStyle(.borderless)
                            .disabled(model.cancelling.contains(booking.id))
                            .accessibilityIdentifier("rental-cancel-\(booking.id)")
                        }
                    }
                    .padding(.vertical, 2)
                }

                if model.empty {
                    Text("Du hast noch nichts gebucht. Such dir im Maschinchenring "
                        + "ein Gerät und frag es für deinen Zeitraum an.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-bookings-empty")
                }
            } footer: {
                Text("Ob eine Anfrage angenommen wird, entscheidet, wem das Gerät "
                    + "gehört. Die Abholadresse steht hier, sobald zugesagt wurde.")
            }
        }
        .accessibilityIdentifier("rental-bookings")
        .refreshable { await model.refresh() }
        .alert(
            "Buchung zurückziehen?",
            isPresented: Binding(
                get: { aboutToCancel != nil },
                set: { if !$0 { aboutToCancel = nil } }
            ),
            presenting: aboutToCancel
        ) { booking in
            Button("Zurückziehen", role: .destructive) {
                Task { await model.cancel(booking) }
            }
            Button("Behalten", role: .cancel) {}
        } message: { booking in
            Text("\(booking.deviceName), \(booking.periodText)")
        }
    }

    private func signIn() async {
        signingIn = true
        defer { signingIn = false }
        // A full sign-in, not a refresh: the rental platform only lands in the
        // token's audience when the authorization request asks for it.
        if await umgebung.anmeldung.anmelden() == .erfolg {
            model.clearTrouble()
            await umgebung.ichLaden()
            await model.refresh()
        }
    }
}

/// One booking, in the list and after it was made.
struct RentalBookingRow: View {
    let booking: RentalBooking

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(booking.deviceName)
                .font(.headline)

            Label(booking.periodText, systemImage: "calendar")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Label(booking.statusLabel, systemImage: booking.state.symbol)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(colour)

            if let price = booking.priceText {
                Text(price)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            // Personal data only ever appears once the platform hands it
            // over, which it does for confirmed bookings of one's own.
            if let address = booking.pickupAddress {
                Label(address, systemImage: "mappin.and.ellipse")
                    .font(.subheadline)
            }
            if let owner = booking.ownerName {
                Label(owner, systemImage: "person")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if let phone = booking.ownerPhone {
                Label(phone, systemImage: "phone")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if let note = booking.note {
                Text(note)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("rental-booking-\(booking.id)")
    }

    /// Colour follows the state, and nothing else does — the wording belongs
    /// to the platform.
    private var colour: Color {
        switch booking.state {
        case .approved: return .green
        case .rejected, .cancelled: return .secondary
        case .pending, .other: return .primary
        }
    }
}
